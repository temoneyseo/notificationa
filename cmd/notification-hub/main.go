package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/adapters/discord"
	"github.com/user/notification-hub/internal/adapters/telegram"
	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/config"
	"github.com/user/notification-hub/internal/http/handlers"
	"github.com/user/notification-hub/internal/security"
	"github.com/user/notification-hub/internal/server"
	"github.com/user/notification-hub/internal/service"
	"github.com/user/notification-hub/internal/storage/sqlite"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		return err
	}
	cipher, err := security.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return err
	}

	messages := sqlite.NewMessageRepository(db)
	channels := service.NewConfigChannelStore(cfg.DomainChannels())
	webhooks := sqlite.NewWebhookRepository(db)
	registry := service.NewAdapterRegistry()
	registry.Register(telegram.New(telegram.AdapterConfig{HTTPClient: &http.Client{Timeout: 15 * time.Second}}))
	registry.Register(discord.New(discord.AdapterConfig{HTTPClient: &http.Client{Timeout: 15 * time.Second}}))
	processor := ai.SafeProcessor{Processor: ai.NewOpenAIProcessor(ai.OpenAIConfig{
		APIKey:  cfg.OpenAI.APIKey,
		Model:   cfg.OpenAI.Model,
		BaseURL: cfg.OpenAI.BaseURL,
		Timeout: cfg.OpenAI.Timeout,
	})}
	pipeline := service.NewPipeline(service.PipelineDeps{
		Messages: messages,
		Channels: channels,
		Adapters: registry,
		AI:       processor,
		Cipher:   cipher,
	})
	webhookDispatcher := service.NewWebhookDispatcher(service.WebhookDispatcherDeps{Webhooks: webhooks, Cipher: cipher})
	autoReply := service.NewAutoReplyService(channels, pipeline, processor)
	inbound := service.NewInboundService(service.InboundDeps{
		Messages:             messages,
		WebhookDispatcher:    webhookDispatcher,
		AutoReply:            autoReply,
		LogInboundMessages:   cfg.LogInboundMessages,
		InboundMessageWriter: os.Stdout,
	})
	listenerCtx, stopListeners := context.WithCancel(context.Background())
	defer stopListeners()
	startInboundListeners(listenerCtx, channels, inbound)

	srv := server.New(cfg.HTTPAddr, server.Dependencies{
		RegisterHandlers: func(router *gin.Engine) {
			handlers.RegisterRoutes(router, handlers.Dependencies{
				Messages: messages,
				Channels: channels,
				Webhooks: webhooks,
				Pipeline: pipeline,
				Cipher:   cipher,
			})
		},
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

func startInboundListeners(ctx context.Context, channels service.ChannelStore, inbound *service.InboundService) {
	configured, err := channels.ListActive(ctx)
	if err != nil {
		log.Printf("load inbound channels: %v", err)
		return
	}
	for _, channel := range configured {
		decrypted := channel.Config
		switch channel.Platform {
		case "telegram":
			token, _ := decrypted["bot_token"].(string)
			if token == "" {
				continue
			}
			adapter := telegram.New(telegram.AdapterConfig{HTTPClient: &http.Client{Timeout: 35 * time.Second}})
			poller := telegram.NewPoller(adapter, token, inbound)
			go func() {
				if err := poller.Start(ctx); err != nil && ctx.Err() == nil {
					log.Printf("telegram polling stopped: %v", err)
				}
			}()
		case "discord":
			token, _ := decrypted["bot_token"].(string)
			if token == "" {
				continue
			}
			gateway := discord.NewGateway("", token, inbound)
			go func() {
				if err := gateway.Start(ctx); err != nil && ctx.Err() == nil {
					log.Printf("discord gateway stopped: %v", err)
				}
			}()
		}
	}
}
