package db

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BotDatabase struct {
	client     *mongo.Client
	db         *mongo.Database
	connected  bool
	mu         sync.RWMutex
	timeLoc    *time.Location
}

type UserRecord struct {
	UserID    int64     `bson:"user_id"`
	FirstName string    `bson:"first_name"`
	LastName  string    `bson:"last_name"`
	Username  string    `bson:"username"`
	StartedAt time.Time `bson:"started_at"`
}

type AuthGCRecord struct {
	ChatID       int64     `bson:"chat_id"`
	AuthorizedBy int64     `bson:"authorized_by"`
	AuthorizedAt time.Time `bson:"authorized_at"`
}

type PremiumRecord struct {
	UserID     int64     `bson:"user_id"`
	ExpiresAt  time.Time `bson:"expires_at"`
	AddedBy    int64     `bson:"added_by"`
	AddedAt    time.Time `bson:"added_at"`
}

type FSubChannel struct {
	ChannelID int64  `bson:"channel_id"`
	Title     string `bson:"title"`
	InviteURL string `bson:"invite_url"`
}

type TrafficStats struct {
	Today    int64 `json:"today_bytes"`
	ThisWeek int64 `json:"weekly_bytes"`
	ThisMonth int64 `json:"monthly_bytes"`
	ThisYear int64 `json:"yearly_bytes"`
	Overall  int64 `json:"overall_bytes"`
}

func NewBotDatabase(uri string) (*BotDatabase, error) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.UTC
	}

	if uri == "" {
		log.Println("[MongoDB] No DATABASE_URL provided. Running with in-memory fallback for bot settings.")
		return &BotDatabase{connected: false, timeLoc: loc}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Printf("[MongoDB] Ping warning: %v (continuing with degraded DB connectivity)", err)
	} else {
		log.Println("[MongoDB] Connected successfully to MongoDB Atlas")
	}

	database := client.Database("filetolink_db")
	return &BotDatabase{
		client:    client,
		db:        database,
		connected: true,
		timeLoc:   loc,
	}, nil
}

// -----------------------------------------------------------------------------
// Bandwidth Tracking: Today, Weekly, Monthly, Yearly, Overall
// -----------------------------------------------------------------------------

// RecordStreamTransfer atomically increments the transferred bytes across all time windows.
func (b *BotDatabase) RecordStreamTransfer(bytesTransferred int64) {
	if !b.connected || bytesTransferred <= 0 {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		now := time.Now().In(b.timeLoc)
		todayKey := now.Format("2006-01-02")
		year, week := now.ISOWeek()
		weekKey := fmt.Sprintf("%d-W%02d", year, week)
		monthKey := now.Format("2006-01")
		yearKey := now.Format("2006")

		col := b.db.Collection("traffic_stats")
		
		// 1. Daily
		_, _ = col.UpdateOne(ctx,
			bson.M{"type": "daily", "key": todayKey},
			bson.M{"$inc": bson.M{"bytes": bytesTransferred}, "$setOnInsert": bson.M{"created_at": now}},
			options.Update().SetUpsert(true),
		)

		// 2. Weekly
		_, _ = col.UpdateOne(ctx,
			bson.M{"type": "weekly", "key": weekKey},
			bson.M{"$inc": bson.M{"bytes": bytesTransferred}},
			options.Update().SetUpsert(true),
		)

		// 3. Monthly
		_, _ = col.UpdateOne(ctx,
			bson.M{"type": "monthly", "key": monthKey},
			bson.M{"$inc": bson.M{"bytes": bytesTransferred}},
			options.Update().SetUpsert(true),
		)

		// 4. Yearly
		_, _ = col.UpdateOne(ctx,
			bson.M{"type": "yearly", "key": yearKey},
			bson.M{"$inc": bson.M{"bytes": bytesTransferred}},
			options.Update().SetUpsert(true),
		)

		// 5. Overall
		_, _ = col.UpdateOne(ctx,
			bson.M{"type": "overall", "key": "all_time"},
			bson.M{"$inc": bson.M{"bytes": bytesTransferred}},
			options.Update().SetUpsert(true),
		)
	}()
}

func (b *BotDatabase) GetTrafficStats(ctx context.Context) (*TrafficStats, error) {
	stats := &TrafficStats{}
	if !b.connected {
		return stats, nil
	}

	col := b.db.Collection("traffic_stats")
	now := time.Now().In(b.timeLoc)
	todayKey := now.Format("2006-01-02")
	year, week := now.ISOWeek()
	weekKey := fmt.Sprintf("%d-W%02d", year, week)
	monthKey := now.Format("2006-01")
	yearKey := now.Format("2006")

	type resDoc struct {
		Bytes int64 `bson:"bytes"`
	}

	var d resDoc
	if err := col.FindOne(ctx, bson.M{"type": "daily", "key": todayKey}).Decode(&d); err == nil {
		stats.Today = d.Bytes
	}

	d.Bytes = 0
	if err := col.FindOne(ctx, bson.M{"type": "weekly", "key": weekKey}).Decode(&d); err == nil {
		stats.ThisWeek = d.Bytes
	}

	d.Bytes = 0
	if err := col.FindOne(ctx, bson.M{"type": "monthly", "key": monthKey}).Decode(&d); err == nil {
		stats.ThisMonth = d.Bytes
	}

	d.Bytes = 0
	if err := col.FindOne(ctx, bson.M{"type": "yearly", "key": yearKey}).Decode(&d); err == nil {
		stats.ThisYear = d.Bytes
	}

	d.Bytes = 0
	if err := col.FindOne(ctx, bson.M{"type": "overall", "key": "all_time"}).Decode(&d); err == nil {
		stats.Overall = d.Bytes
	}

	return stats, nil
}

// -----------------------------------------------------------------------------
// User Tracking & Start in DM
// -----------------------------------------------------------------------------

func (b *BotDatabase) SaveUserStart(ctx context.Context, userID int64, firstName, lastName, username string) error {
	if !b.connected || userID == 0 {
		return nil
	}
	col := b.db.Collection("users")
	_, err := col.UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{
			"$set": bson.M{
				"first_name": firstName,
				"last_name":  lastName,
				"username":   username,
			},
			"$setOnInsert": bson.M{
				"user_id":    userID,
				"started_at": time.Now(),
			},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (b *BotDatabase) HasUserStarted(ctx context.Context, userID int64) bool {
	if !b.connected || userID == 0 {
		return true // Default permit if DB offline
	}
	col := b.db.Collection("users")
	count, err := col.CountDocuments(ctx, bson.M{"user_id": userID})
	return err == nil && count > 0
}

func (b *BotDatabase) TotalUsers(ctx context.Context) int64 {
	if !b.connected {
		return 0
	}
	count, _ := b.db.Collection("users").CountDocuments(ctx, bson.M{})
	return count
}

// -----------------------------------------------------------------------------
// Authorized Group Chats (Auth GC)
// -----------------------------------------------------------------------------

func (b *BotDatabase) IsGCAuthorized(ctx context.Context, chatID int64) bool {
	if !b.connected {
		return false
	}
	col := b.db.Collection("authorized_gc")
	count, err := col.CountDocuments(ctx, bson.M{"chat_id": chatID})
	return err == nil && count > 0
}

func (b *BotDatabase) AuthorizeGC(ctx context.Context, chatID int64, authorizedBy int64) error {
	if !b.connected {
		return fmt.Errorf("database not connected")
	}
	col := b.db.Collection("authorized_gc")
	_, err := col.UpdateOne(ctx,
		bson.M{"chat_id": chatID},
		bson.M{"$set": bson.M{"chat_id": chatID, "authorized_by": authorizedBy, "authorized_at": time.Now()}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (b *BotDatabase) DeauthorizeGC(ctx context.Context, chatID int64) error {
	if !b.connected {
		return fmt.Errorf("database not connected")
	}
	col := b.db.Collection("authorized_gc")
	_, err := col.DeleteOne(ctx, bson.M{"chat_id": chatID})
	return err
}

func (b *BotDatabase) ListAuthorizedGCs(ctx context.Context) ([]AuthGCRecord, error) {
	if !b.connected {
		return nil, nil
	}
	col := b.db.Collection("authorized_gc")
	cursor, err := col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []AuthGCRecord
	err = cursor.All(ctx, &list)
	return list, err
}

// -----------------------------------------------------------------------------
// Paid / Premium Subscriptions
// -----------------------------------------------------------------------------

func (b *BotDatabase) IsPremiumUser(ctx context.Context, userID int64) bool {
	if !b.connected {
		return false
	}
	col := b.db.Collection("premium_users")
	var rec PremiumRecord
	err := col.FindOne(ctx, bson.M{"user_id": userID}).Decode(&rec)
	if err != nil {
		return false
	}
	return time.Now().Before(rec.ExpiresAt)
}

func (b *BotDatabase) GetPremiumPlan(ctx context.Context, userID int64) (*PremiumRecord, bool) {
	if !b.connected {
		return nil, false
	}
	col := b.db.Collection("premium_users")
	var rec PremiumRecord
	err := col.FindOne(ctx, bson.M{"user_id": userID}).Decode(&rec)
	if err != nil {
		return nil, false
	}
	if time.Now().After(rec.ExpiresAt) {
		return nil, false
	}
	return &rec, true
}

func (b *BotDatabase) AddPremium(ctx context.Context, userID int64, expiresAt time.Time, addedBy int64) error {
	if !b.connected {
		return fmt.Errorf("database not connected")
	}
	col := b.db.Collection("premium_users")
	_, err := col.UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": bson.M{
			"user_id":    userID,
			"expires_at": expiresAt,
			"added_by":   addedBy,
			"added_at":   time.Now(),
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (b *BotDatabase) RemovePremium(ctx context.Context, userID int64) error {
	if !b.connected {
		return fmt.Errorf("database not connected")
	}
	col := b.db.Collection("premium_users")
	_, err := col.DeleteOne(ctx, bson.M{"user_id": userID})
	return err
}

func (b *BotDatabase) ListPremiumUsers(ctx context.Context) ([]PremiumRecord, error) {
	if !b.connected {
		return nil, nil
	}
	col := b.db.Collection("premium_users")
	cursor, err := col.Find(ctx, bson.M{"expires_at": bson.M{"$gt": time.Now()}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []PremiumRecord
	err = cursor.All(ctx, &list)
	return list, err
}

// -----------------------------------------------------------------------------
// PM Mode & Bot Settings
// -----------------------------------------------------------------------------

func (b *BotDatabase) GetPMMode(ctx context.Context, defaultVal bool) bool {
	if !b.connected {
		return defaultVal
	}
	col := b.db.Collection("bot_settings")
	var res struct {
		Enabled bool `bson:"enabled"`
	}
	err := col.FindOne(ctx, bson.M{"setting": "pm_mode"}).Decode(&res)
	if err != nil {
		return defaultVal
	}
	return res.Enabled
}

func (b *BotDatabase) SetPMMode(ctx context.Context, enabled bool) error {
	if !b.connected {
		return fmt.Errorf("database not connected")
	}
	col := b.db.Collection("bot_settings")
	_, err := col.UpdateOne(ctx,
		bson.M{"setting": "pm_mode"},
		bson.M{"$set": bson.M{"setting": "pm_mode", "enabled": enabled, "updated_at": time.Now()}},
		options.Update().SetUpsert(true),
	)
	return err
}

// -----------------------------------------------------------------------------
// Dynamic Force Sub (FSub) Channels
// -----------------------------------------------------------------------------

func (b *BotDatabase) AddFSubChannel(ctx context.Context, ch FSubChannel) error {
	if !b.connected {
		return fmt.Errorf("database not connected")
	}
	col := b.db.Collection("fsub_channels")
	_, err := col.UpdateOne(ctx,
		bson.M{"channel_id": ch.ChannelID},
		bson.M{"$set": ch},
		options.Update().SetUpsert(true),
	)
	return err
}

func (b *BotDatabase) RemoveFSubChannel(ctx context.Context, channelID int64) error {
	if !b.connected {
		return fmt.Errorf("database not connected")
	}
	col := b.db.Collection("fsub_channels")
	_, err := col.DeleteOne(ctx, bson.M{"channel_id": channelID})
	return err
}

func (b *BotDatabase) ListFSubChannels(ctx context.Context) ([]FSubChannel, error) {
	if !b.connected {
		return nil, nil
	}
	col := b.db.Collection("fsub_channels")
	cursor, err := col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []FSubChannel
	err = cursor.All(ctx, &list)
	return list, err
}

// -----------------------------------------------------------------------------
// Reel Channel Media Storage
// -----------------------------------------------------------------------------

func (b *BotDatabase) SaveReelMedia(ctx context.Context, messageID int, fileType string) error {
	if !b.connected || messageID == 0 {
		return nil
	}
	col := b.db.Collection("reel_media")
	_, err := col.UpdateOne(ctx,
		bson.M{"message_id": messageID},
		bson.M{"$set": bson.M{
			"message_id": messageID,
			"file_type":  fileType,
			"created_at": time.Now(),
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

// -----------------------------------------------------------------------------
// Restart Notification Message Tracking
// -----------------------------------------------------------------------------

func (b *BotDatabase) SaveRestartMessage(ctx context.Context, messageID, chatID int64) error {
	if !b.connected {
		return nil
	}
	col := b.db.Collection("restart_message")
	_, err := col.UpdateOne(ctx,
		bson.M{"type": "restart"},
		bson.M{"$set": bson.M{"message_id": messageID, "chat_id": chatID, "timestamp": time.Now()}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (b *BotDatabase) GetRestartMessage(ctx context.Context) (int64, int64, error) {
	if !b.connected {
		return 0, 0, fmt.Errorf("db not connected")
	}
	col := b.db.Collection("restart_message")
	var doc struct {
		MessageID int64 `bson:"message_id"`
		ChatID    int64 `bson:"chat_id"`
	}
	err := col.FindOne(ctx, bson.M{"type": "restart"}).Decode(&doc)
	if err != nil {
		return 0, 0, err
	}
	return doc.MessageID, doc.ChatID, nil
}

func (b *BotDatabase) DeleteRestartMessage(ctx context.Context) error {
	if !b.connected {
		return nil
	}
	col := b.db.Collection("restart_message")
	_, err := col.DeleteMany(ctx, bson.M{"type": "restart"})
	return err
}

// -----------------------------------------------------------------------------
// User Ban Management
// -----------------------------------------------------------------------------

type BannedRecord struct {
	UserID   int64     `bson:"user_id"`
	Reason   string    `bson:"reason"`
	BannedAt time.Time `bson:"banned_at"`
}

func (b *BotDatabase) BanUser(ctx context.Context, userID int64, reason string) error {
	if !b.connected || userID == 0 {
		return nil
	}
	col := b.db.Collection("banned_users")
	_, err := col.UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": bson.M{
			"user_id":   userID,
			"reason":    reason,
			"banned_at": time.Now(),
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (b *BotDatabase) UnbanUser(ctx context.Context, userID int64) error {
	if !b.connected || userID == 0 {
		return nil
	}
	col := b.db.Collection("banned_users")
	_, err := col.DeleteOne(ctx, bson.M{"user_id": userID})
	return err
}

func (b *BotDatabase) IsBanned(ctx context.Context, userID int64) bool {
	if !b.connected || userID == 0 {
		return false
	}
	col := b.db.Collection("banned_users")
	count, err := col.CountDocuments(ctx, bson.M{"user_id": userID})
	return err == nil && count > 0
}

func (b *BotDatabase) ListBannedUsers(ctx context.Context) ([]BannedRecord, error) {
	if !b.connected {
		return nil, nil
	}
	col := b.db.Collection("banned_users")
	cursor, err := col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []BannedRecord
	err = cursor.All(ctx, &list)
	return list, err
}
