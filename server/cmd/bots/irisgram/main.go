// Command irisgram is the DeltaGram group moderation bot (Russian UI).
//
// It connects to the local DeltaGram MTProto server as an external bot, using
// the token issued by the DeltaGram Admin API, and moderates supergroups where
// it has been added and promoted.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/gotd/log/logzap"
	"github.com/iamxvbaba/td/exchange"
	"github.com/iamxvbaba/td/mtproxy"
	"github.com/iamxvbaba/td/mtproxy/obfuscator"
	"github.com/iamxvbaba/td/proto/codec"
	"github.com/iamxvbaba/td/telegram"
	"github.com/iamxvbaba/td/telegram/dcs"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/transport"
	"go.uber.org/zap"

	"telesrv/internal/botdata"
	"telesrv/internal/mtprotoedge"
)

// obfuscatedResolver dials the local server with standard secretless MTProto
// TCP obfuscation (obfuscated2), matching the production server path.
type obfuscatedResolver struct {
	host string
	port int
}

func (r obfuscatedResolver) dial(ctx context.Context, dc int) (transport.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(r.host, strconv.Itoa(r.port)))
	if err != nil {
		return nil, err
	}
	obf := obfuscator.Obfuscated2(rand.Reader, conn)
	if err := obf.Handshake(codec.IntermediateClientStart, dc, mtproxy.Secret{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("obfuscated2 handshake: %w", err)
	}
	proto := transport.NewProtocol(func() transport.Codec { return codec.NoHeader{Codec: codec.Intermediate{}} })
	tc, err := proto.Handshake(obf)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("transport handshake: %w", err)
	}
	return tc, nil
}

func (r obfuscatedResolver) Primary(ctx context.Context, dc int, _ dcs.List) (transport.Conn, error) {
	return r.dial(ctx, dc)
}
func (r obfuscatedResolver) MediaOnly(ctx context.Context, dc int, _ dcs.List) (transport.Conn, error) {
	return r.dial(ctx, dc)
}
func (r obfuscatedResolver) CDN(ctx context.Context, dc int, _ dcs.List) (transport.Conn, error) {
	return r.dial(ctx, dc)
}

func main() {
	addr := flag.String("addr", envOr("IRISGRAM_ADDR", "127.0.0.1:2398"), "DeltaGram MTProto address")
	dcID := flag.Int("dc", envInt("IRISGRAM_DC", 2), "DC id")
	token := flag.String("token", os.Getenv("IRISGRAM_BOT_TOKEN"), "bot token <bot_id>:<secret>")
	rsaPath := flag.String("rsa", envOr("IRISGRAM_RSA_KEY", "/server_state/server_rsa.pem"), "server RSA public key path")
	statePath := flag.String("state", envOr("IRISGRAM_STATE", "/var/lib/irisgram/state.json"), "JSON state path")
	apiID := flag.Int("api-id", envInt("IRISGRAM_API_ID", 1), "api_id")
	apiHash := flag.String("api-hash", envOr("IRISGRAM_API_HASH", "hash"), "api_hash")
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "irisgram: IRISGRAM_BOT_TOKEN is required")
		os.Exit(2)
	}
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	store, err := OpenStore(*statePath)
	if err != nil {
		logger.Fatal("open state", zap.Error(err))
	}

	priv, err := mtprotoedge.LoadOrGenerateRSAKey(*rsaPath)
	if err != nil {
		logger.Fatal("load RSA key", zap.Error(err))
	}
	host, portStr, err := net.SplitHostPort(*addr)
	if err != nil {
		logger.Fatal("parse address", zap.Error(err))
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		logger.Fatal("parse port", zap.Error(err))
	}

	bot := &Bot{store: store, log: logger, cache: newPeerCache()}

	// Personal stats need cross-user data that MTProto deliberately hides from
	// bots, so they come from a read-only database connection. The bot keeps
	// working (with a shorter reply) when the DSN is not configured.
	if dsn := strings.TrimSpace(os.Getenv("IRISGRAM_POSTGRES_DSN")); dsn != "" {
		reader, err := botdata.Open(context.Background(), dsn)
		if err != nil {
			logger.Warn("open stats reader", zap.Error(err))
		} else {
			bot.data = reader
			defer reader.Close()
		}
	}

	client := telegram.NewClient(*apiID, *apiHash, telegram.Options{
		PublicKeys: []exchange.PublicKey{{RSA: &priv.PublicKey}},
		Resolver:   obfuscatedResolver{host: host, port: port},
		DCList:     dcs.List{Options: []tg.DCOption{{ID: *dcID, IPAddress: host, Port: port, Static: true}}},
		Logger:     logzap.New(logger.Named("client")),
		UpdateHandler: telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
			return bot.Handle(ctx, u)
		}),
	})
	bot.api = tg.NewClient(client)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.Run(ctx, func(ctx context.Context) error {
		self, err := loginBot(ctx, bot.api, *apiID, *apiHash, *token)
		if err != nil {
			return err
		}
		bot.setSelf(self)
		logger.Info("irisgram started",
			zap.Int64("bot_id", self.ID),
			zap.String("username", self.Username),
		)
		bot.registerCommands(ctx)
		<-ctx.Done()
		return nil
	}); err != nil && ctx.Err() == nil {
		logger.Fatal("run", zap.Error(err))
	}
	logger.Info("irisgram stopped")
}

func loginBot(ctx context.Context, api *tg.Client, apiID int, apiHash, token string) (*tg.User, error) {
	authz, err := api.AuthImportBotAuthorization(ctx, &tg.AuthImportBotAuthorizationRequest{
		APIID: apiID, APIHash: apiHash, BotAuthToken: token,
	})
	if err != nil {
		return nil, fmt.Errorf("importBotAuthorization: %w", err)
	}
	a, ok := authz.(*tg.AuthAuthorization)
	if !ok {
		return nil, fmt.Errorf("authorization type = %T, want *tg.AuthAuthorization", authz)
	}
	self, ok := a.User.(*tg.User)
	if !ok || !self.Bot {
		return nil, fmt.Errorf("logged account is not a bot: %T", a.User)
	}
	return self, nil
}

func (b *Bot) registerCommands(ctx context.Context) {
	if b.api == nil {
		return
	}
	commands := []tg.BotCommand{
		{Command: "ban", Description: "Забанить пользователя"},
		{Command: "unban", Description: "Разбанить пользователя"},
		{Command: "kick", Description: "Кикнуть из чата"},
		{Command: "mute", Description: "Замутить пользователя"},
		{Command: "unmute", Description: "Снять мут"},
		{Command: "warn", Description: "Выдать предупреждение"},
		{Command: "unwarn", Description: "Снять предупреждение"},
		{Command: "warns", Description: "Предупреждения пользователя"},
		{Command: "userid", Description: "ID DeltaGram пользователя из ответа"},
		{Command: "banlist", Description: "Список забаненных"},
		{Command: "admins", Description: "Администрация чата"},
		{Command: "settings", Description: "Настройки модерации"},
		{Command: "rules", Description: "Правила чата"},
		{Command: "stats", Description: "Личная статистика: звёзды, подарки, сообщения"},
		{Command: "help", Description: "Список команд"},
	}
	if _, err := b.api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		Commands: commands,
	}); err != nil {
		b.log.Warn("set bot commands", zap.Error(err))
	}
}

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
