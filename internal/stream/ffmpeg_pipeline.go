package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
)

type TrackInfo struct {
	Index     int    `json:"index"`
	Type      string `json:"type"` // "audio" or "subtitle"
	Codec     string `json:"codec"`
	Language  string `json:"language,omitempty"`
	Title     string `json:"title,omitempty"`
	Channels  int    `json:"channels,omitempty"`
}

type MediaTracks struct {
	AudioTracks    []TrackInfo `json:"audio_tracks"`
	SubtitleTracks []TrackInfo `json:"subtitle_tracks"`
}

// ProbeMediaTracks runs ffprobe on the input stream to discover available audio and subtitle tracks.
func ProbeMediaTracks(ctx context.Context, inputURL string) (*MediaTracks, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		inputURL,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probeData struct {
		Streams []struct {
			Index     int               `json:"index"`
			CodecType string            `json:"codec_type"`
			CodecName string            `json:"codec_name"`
			Channels  int               `json:"channels"`
			Tags      map[string]string `json:"tags"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(out, &probeData); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe json: %w", err)
	}

	tracks := &MediaTracks{
		AudioTracks:    make([]TrackInfo, 0),
		SubtitleTracks: make([]TrackInfo, 0),
	}

	audioIdx := 0
	subIdx := 0
	for _, s := range probeData.Streams {
		lang := ""
		title := ""
		if s.Tags != nil {
			lang = s.Tags["language"]
			title = s.Tags["title"]
		}
		if title == "" {
			if lang != "" {
				title = fmt.Sprintf("Track %d (%s)", s.Index, lang)
			} else {
				title = fmt.Sprintf("Track %d", s.Index)
			}
		}

		if s.CodecType == "audio" {
			tracks.AudioTracks = append(tracks.AudioTracks, TrackInfo{
				Index:    audioIdx,
				Type:     "audio",
				Codec:    s.CodecName,
				Language: lang,
				Title:    title,
				Channels: s.Channels,
			})
			audioIdx++
		} else if s.CodecType == "subtitle" {
			tracks.SubtitleTracks = append(tracks.SubtitleTracks, TrackInfo{
				Index:    subIdx,
				Type:     "subtitle",
				Codec:    s.CodecName,
				Language: lang,
				Title:    title,
			})
			subIdx++
		}
	}

	return tracks, nil
}

// StreamRemuxWithFFmpeg pipes video remuxed with specific audio/sub track to http.ResponseWriter
func StreamRemuxWithFFmpeg(
	ctx context.Context,
	w http.ResponseWriter,
	inputURL string,
	audioIndex int,
	subIndex int,
	seekSeconds float64,
	fileName string,
) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
	}

	if seekSeconds > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSeconds))
	}

	args = append(args, "-i", inputURL, "-map", "0:v:0")

	if audioIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", audioIndex))
	} else {
		args = append(args, "-map", "0:a:0?")
	}

	// Copy video codec, transcode audio to standard stereo AAC for universal browser playback
	args = append(args,
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-ac", "2",
	)

	if subIndex >= 0 {
		args = append(args, "-map", fmt.Sprintf("0:s:%d", subIndex), "-c:s", "mov_text")
	}

	args = append(args,
		"-f", "mp4",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"pipe:1",
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe failed: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start failed: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileName))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Accept-Ranges", "none")

	buf := make([]byte, 128*1024)
	for {
		n, rErr := stdout.Read(buf)
		if n > 0 {
			if _, wErr := w.Write(buf[:n]); wErr != nil {
				return wErr
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			return rErr
		}
	}

	return nil
}

// StreamSubtitleTrack streams extracted subtitles in WebVTT or ASS format
func StreamSubtitleTrack(
	ctx context.Context,
	w http.ResponseWriter,
	inputURL string,
	subIndex int,
	rawFormat string,
) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputURL,
		"-map", fmt.Sprintf("0:s:%d", subIndex),
	}

	contentType := "text/vtt"
	if rawFormat == "ass" || rawFormat == "raw" {
		args = append(args, "-c:s", "copy", "-f", "ass", "pipe:1")
		contentType = "text/x-ssa"
	} else {
		args = append(args, "-f", "webvtt", "pipe:1")
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Access-Control-Allow-Origin", "*")

	buf := make([]byte, 64*1024)
	for {
		n, rErr := stdout.Read(buf)
		if n > 0 {
			if _, wErr := w.Write(buf[:n]); wErr != nil {
				return wErr
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if rErr != nil {
			break
		}
	}
	return nil
}

// Suppress unused imports
var _ = strconv.Itoa
var _ = log.Printf
