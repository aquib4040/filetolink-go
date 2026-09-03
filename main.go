package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"filetolink-go/internal/bot"
	"filetolink-go/internal/config"
	"filetolink-go/internal/db"
	"filetolink-go/internal/pool"
	"filetolink-go/internal/stream"
)

func main() {
	log.Println("======================================================")
	log.Println("⚡ High-Performance Cloud MTProto Streaming Service")
	log.Println("======================================================")

	// 1. Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("[Fatal] Configuration error: %v", err)
	}

	_ = os.MkdirAll(cfg.GotdDataPath, 0755)

	// 2. Initialize Database (MongoDB)
	database, err := db.NewBotDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Printf("[Database] Notice: %v", err)
	}

	// 3. Initialize MTProto Session Pool
	sessPool := pool.NewSessionPool(cfg.GotdDataPath)

	// 4. Pre-authenticate multi-tokens in background with rate-limiting
	if len(cfg.MultiTokens) > 0 {
		sessPool.WarmupTokens(int(cfg.APIID), cfg.APIHash, cfg.MultiTokens)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 5. Start HTTP Streaming & REST Server
	httpServer := stream.NewHTTPServer(cfg, sessPool, database)
	go func() {
		if err := httpServer.Start(ctx); err != nil {
			log.Fatalf("[HTTPServer] Fatal error: %v", err)
		}
	}()

	// 6. Start Telegram Bot Service
	botManager := bot.NewBotManager(cfg, sessPool, database)
	go func() {
		if err := botManager.Start(ctx); err != nil {
			log.Printf("[BotManager] Notice: %v", err)
		}
	}()

	// 7. Wait for termination signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("[Main] Shutting down gracefully...")
	sessPool.StopAll()
	cancel()
	time.Sleep(1 * time.Second)
	log.Println("[Main] Goodbye!")
}
