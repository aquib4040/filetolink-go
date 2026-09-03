package stream

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"filetolink-go/internal/pool"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

type ParallelFetcher struct {
	pool *pool.SessionPool
}

func NewParallelFetcher(p *pool.SessionPool) *ParallelFetcher {
	return &ParallelFetcher{pool: p}
}

func getFileWithRetry(ctx context.Context, api *tg.Client, location tg.InputFileLocationClass, offset int64, limit int) ([]byte, error) {
	for attempt := 0; attempt < 10; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		getResp, err := api.UploadGetFile(ctx, &tg.UploadGetFileRequest{
			Location: location,
			Offset:   offset,
			Limit:    limit,
		})
		if err == nil {
			if uFile, ok := getResp.(*tg.UploadFile); ok {
				return uFile.Bytes, nil
			}
			return nil, fmt.Errorf("unexpected response type from UploadGetFile")
		}

		if d, ok := tgerr.AsFloodWait(err); ok {
			waitTime := d
			if waitTime < 1*time.Second {
				waitTime = 1 * time.Second
			}
			time.Sleep(waitTime + time.Duration(rand.Intn(400))*time.Millisecond)
			continue
		}

		if strings.Contains(err.Error(), "FLOOD_WAIT") {
			time.Sleep(2*time.Second + time.Duration(rand.Intn(400))*time.Millisecond)
			continue
		}

		if strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "reset") ||
			strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "broken pipe") ||
			strings.Contains(err.Error(), "write tcp") || strings.Contains(err.Error(), "read tcp") {
			time.Sleep(400 * time.Millisecond)
			continue
		}

		return nil, err
	}
	return nil, fmt.Errorf("exceeded max retries for offset %d", offset)
}

func (pf *ParallelFetcher) ResolveDocumentWithBot(
	ctx context.Context,
	bot *pool.BotSession,
	chatID int64,
	messageID int64,
) (int64, string, tg.InputFileLocationClass, error) {
	inputChannel, err := bot.ResolveChannel(ctx, chatID)
	if err != nil {
		return 0, "", nil, fmt.Errorf("failed to resolve channel %d: %w", chatID, err)
	}

	res, err := bot.API.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: inputChannel,
		ID: []tg.InputMessageClass{
			&tg.InputMessageID{ID: int(messageID)},
		},
	})
	if err != nil {
		return 0, "", nil, fmt.Errorf("ChannelsGetMessages failed: %w", err)
	}

	var location tg.InputFileLocationClass
	var totalSize int64
	var fileName string

	switch m := res.(type) {
	case *tg.MessagesChannelMessages:
		for _, msgClass := range m.Messages {
			if msg, ok := msgClass.(*tg.Message); ok {
				if media, ok := msg.Media.(*tg.MessageMediaDocument); ok {
					if doc, ok := media.Document.(*tg.Document); ok {
						totalSize = doc.Size
						location = &tg.InputDocumentFileLocation{
							ID:            doc.ID,
							AccessHash:    doc.AccessHash,
							FileReference: doc.FileReference,
						}
						for _, attr := range doc.Attributes {
							if fNameAttr, ok := attr.(*tg.DocumentAttributeFilename); ok {
								fileName = fNameAttr.FileName
								break
							}
						}
					}
				}
			}
		}
	}

	if location == nil {
		return 0, "", nil, fmt.Errorf("document location not found for msg %d in channel %d", messageID, chatID)
	}

	return totalSize, fileName, location, nil
}

// StreamMessageRange streams chunks in parallel from Telegram MTProto to the response writer.
func (pf *ParallelFetcher) StreamMessageRange(
	ctx context.Context,
	apiID int32,
	apiHash string,
	botTokens []string,
	chatID int64,
	messageID int64,
	startByte int64,
	endByte int64,
	totalSize int64,
	w io.Writer,
	onByteProgress func(n int64),
) error {
	bot, err := pf.pool.GetNextAvailable(botTokens, int(apiID), apiHash)
	if err != nil {
		return fmt.Errorf("no bot session available for download: %w", err)
	}

	bot.StartDownload()
	defer bot.EndDownload()

	resolvedSize, _, resolvedLoc, resolveErr := pf.ResolveDocumentWithBot(ctx, bot, chatID, messageID)
	if resolveErr != nil {
		return fmt.Errorf("failed to resolve document for download: %w", resolveErr)
	}
	location := resolvedLoc
	if totalSize <= 0 {
		totalSize = resolvedSize
	}

	if endByte < 0 || endByte >= totalSize {
		endByte = totalSize - 1
	}

	chunkSize := int64(1024 * 1024) // 1 MiB Telegram part limit
	alignedStart := (startByte / chunkSize) * chunkSize

	type chunkTask struct {
		index  int
		offset int64
		limit  int
	}

	type chunkResult struct {
		index int
		data  []byte
		err   error
	}

	var tasks []chunkTask
	taskIdx := 0
	for off := alignedStart; off <= endByte; off += chunkSize {
		tasks = append(tasks, chunkTask{index: taskIdx, offset: off, limit: int(chunkSize)})
		taskIdx++
	}

	numTasks := len(tasks)
	if numTasks == 0 {
		return nil
	}

	concurrency := 16
	if envVal := os.Getenv("DOWNLOAD_THREADS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil && val > 0 {
			concurrency = val
		}
	}
	if concurrency > numTasks {
		concurrency = numTasks
	}

	taskChan := make(chan chunkTask, numTasks)
	for _, t := range tasks {
		taskChan <- t
	}
	close(taskChan)

	resChan := make(chan chunkResult, numTasks)

	var refreshMu sync.Mutex
	var lastRefreshed time.Time

	refreshFileReference := func(workerBot *pool.BotSession) error {
		refreshMu.Lock()
		defer refreshMu.Unlock()

		if time.Since(lastRefreshed) < 10*time.Second {
			return nil
		}

		safeSuffix := workerBot.Token
		if len(safeSuffix) > 6 {
			safeSuffix = safeSuffix[len(safeSuffix)-6:]
		}
		log.Printf("[Streamer] File reference expired for msg %d. Refreshing using bot ...%s...", messageID, safeSuffix)
		_, _, newLoc, err := pf.ResolveDocumentWithBot(ctx, workerBot, chatID, messageID)
		if err != nil {
			return fmt.Errorf("failed to refresh document reference: %w", err)
		}

		if newDocLoc, ok := newLoc.(*tg.InputDocumentFileLocation); ok {
			if oldDocLoc, ok := location.(*tg.InputDocumentFileLocation); ok {
				oldDocLoc.FileReference = newDocLoc.FileReference
			}
		}

		lastRefreshed = time.Now()
		return nil
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i := 0; i < concurrency; i++ {
		go func() {
			workerBot := bot
			for t := range taskChan {
				select {
				case <-workerCtx.Done():
					resChan <- chunkResult{index: t.index, err: workerCtx.Err()}
					return
				default:
				}

				workerBot.Touch()
				b, err := getFileWithRetry(workerCtx, workerBot.API, location, t.offset, t.limit)
				if err != nil {
					errStr := err.Error()
					if strings.Contains(errStr, "FILE_REFERENCE_EXPIRED") || strings.Contains(errStr, "FILE_REFERENCE_INVALID") {
						if rErr := refreshFileReference(workerBot); rErr == nil {
							b, err = getFileWithRetry(workerCtx, workerBot.API, location, t.offset, t.limit)
						}
					}
				}

				if err != nil {
					cancel()
					resChan <- chunkResult{index: t.index, err: fmt.Errorf("chunk at offset %d failed: %w", t.offset, err)}
					return
				}

				chunkStart := startByte
				if t.offset > chunkStart {
					chunkStart = t.offset
				}
				chunkEnd := endByte
				if t.offset+chunkSize-1 < chunkEnd {
					chunkEnd = t.offset + chunkSize - 1
				}

				sliceStart := chunkStart - t.offset
				sliceEnd := chunkEnd - t.offset + 1

				if sliceStart < 0 {
					sliceStart = 0
				}
				if sliceEnd > int64(len(b)) {
					sliceEnd = int64(len(b))
				}

				var trimmed []byte
				if sliceStart < sliceEnd {
					trimmed = b[int(sliceStart):int(sliceEnd)]
				}

				resChan <- chunkResult{index: t.index, data: trimmed}
			}
		}()
	}

	results := make(map[int][]byte)
	nextToStream := 0

	for i := 0; i < numTasks; i++ {
		res := <-resChan
		if res.err != nil {
			cancel()
			return res.err
		}
		results[res.index] = res.data

		for {
			data, exists := results[nextToStream]
			if !exists {
				break
			}
			if len(data) > 0 {
				n, wErr := w.Write(data)
				if onByteProgress != nil && n > 0 {
					onByteProgress(int64(n))
				}
				if wErr != nil {
					cancel()
					return wErr
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			delete(results, nextToStream)
			nextToStream++
		}
	}

	return nil
}
