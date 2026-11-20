// Command casinobot is the DeltaGram casino bot (Russian replies).
//
// It is an external MTProto client, like mogbot: it logs in with the bot token
// issued by the DeltaGram Admin API and answers /start, /help and /balance with
// an inline web-view button that opens the casino Mini App.
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
	addr := flag.String("addr", envOr("CASINOBOT_ADDR", "127.0.0.1:2398"), "DeltaGram MTProto address")
	dcID := flag.Int("dc", envInt("CASINOBOT_DC", 2), "DC id")
	token := flag.String("token", os.Getenv("CASINOBOT_BOT_TOKEN"), "bot token <bot_id>:<secret>")
	rsaPath := flag.String("rsa", envOr("CASINOBOT_RSA_KEY", "/server_state/server_rsa.pem"), "server RSA public key path")
	apiID := flag.Int("api-id", envInt("CASINOBOT_API_ID", 1), "api_id")
	apiHash := flag.String("api-hash", envOr("CASINOBOT_API_HASH", "hash"), "api_hash")
	webAppURL := flag.String("webapp", envOr("CASINOBOT_WEBAPP_URL", "https://dltacasino.duckdns.org/"), "casino Mini App URL")
	chipsPerStar := flag.Int64("chips-per-star", envInt64("CASINOBOT_CHIPS_PER_STAR", 100), "chips per star")
	dsn := flag.String("postgres-dsn", os.Getenv("CASINOBOT_POSTGRES_DSN"), "casino Postgres DSN for /balance")
	// notify-user is a deploy smoke test: on start the bot sends its own welcome
	// message with the web-view button to that user id and then keeps running.
	// It verifies the outbound path (markup round-trip included) without needing
	// a client to press /start.
	notifyUser := flag.Int64("notify-user", envInt64("CASINOBOT_NOTIFY_USER", 0), "send the welcome message to this user id on start")
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "casinobot: CASINOBOT_BOT_TOKEN is required")
		os.Exit(2)
	}
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

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

	bot := newBot(logger, *webAppURL, *chipsPerStar)
	if *dsn != "" {
		if err := bot.openStore(context.Background(), *dsn); err != nil {
			logger.Warn("open casino store", zap.Error(err))
		} else {
			defer bot.closeStore()
		}
	}

	client := telegram.NewClient(*apiID, *apiHash, telegram.Options{
		PublicKeys: []exchange.PublicKey{{RSA: &priv.PublicKey}},
		Resolver:   obfuscatedResolver{host: host, port: port},
		DCList:     dcs.List{Options: []tg.DCOption{{ID: *dcID, IPAddress: host, Port: port, Static: true}}},
		Logger:     logzap.New(logger.Named("client")),
		UpdateHandler: telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
			return bot.handle(ctx, u)
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
		logger.Info("casinobot started",
			zap.Int64("bot_id", self.ID),
			zap.String("username", self.Username),
			zap.String("webapp", *webAppURL),
			zap.Bool("store", bot.hasStore()))
		bot.registerCommands(ctx)
		if *notifyUser != 0 {
			if err := bot.send(ctx, *notifyUser, startText(), bot.openMarkup()); err != nil {
				logger.Warn("casinobot notify failed", zap.Int64("user", *notifyUser), zap.Error(err))
			} else {
				logger.Info("casinobot notify sent", zap.Int64("user", *notifyUser))
			}
		}
		<-ctx.Done()
		return nil
	}); err != nil && ctx.Err() == nil {
		logger.Fatal("run", zap.Error(err))
	}
	logger.Info("casinobot stopped")
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

// registerCommands publishes the Ukrainian command list in the bot's chat menu.
func (b *Bot) registerCommands(ctx context.Context) {
	if b.api == nil {
		return
	}
	commands := []tg.BotCommand{
		{Command: "start", Description: "Открыть казино"},
		{Command: "help", Description: "Правила и выплаты"},
		{Command: "balance", Description: "Мой баланс"},
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

func envInt64(name string, def int64) int64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
