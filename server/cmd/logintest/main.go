// Command logintest calls auth.sendCode against the live DeltaGram server from
// a fresh MTProto connection. It is a development diagnostic for the login
// flood and OTP delivery, not a production component.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

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

type resolver struct {
	host string
	port int
}

func (r resolver) dial(ctx context.Context, dc int) (transport.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(r.host, strconv.Itoa(r.port)))
	if err != nil {
		return nil, err
	}
	obf := obfuscator.Obfuscated2(rand.Reader, conn)
	if err := obf.Handshake(codec.IntermediateClientStart, dc, mtproxy.Secret{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("obfuscated2: %w", err)
	}
	proto := transport.NewProtocol(func() transport.Codec { return codec.NoHeader{Codec: codec.Intermediate{}} })
	tc, err := proto.Handshake(obf)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("transport: %w", err)
	}
	return tc, nil
}

func (r resolver) Primary(ctx context.Context, dc int, _ dcs.List) (transport.Conn, error) {
	return r.dial(ctx, dc)
}
func (r resolver) MediaOnly(ctx context.Context, dc int, _ dcs.List) (transport.Conn, error) {
	return r.dial(ctx, dc)
}
func (r resolver) CDN(ctx context.Context, dc int, _ dcs.List) (transport.Conn, error) {
	return r.dial(ctx, dc)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:2398", "server address")
	rsaPath := flag.String("rsa", "data/server_rsa.pem", "server RSA key")
	phone := flag.String("phone", "", "phone")
	flag.Parse()
	if *phone == "" {
		fmt.Fprintln(os.Stderr, "-phone is required")
		os.Exit(2)
	}
	logger, _ := zap.NewDevelopment()
	priv, err := mtprotoedge.LoadOrGenerateRSAKey(*rsaPath)
	if err != nil {
		logger.Fatal("rsa", zap.Error(err))
	}
	host, portStr, _ := net.SplitHostPort(*addr)
	port, _ := strconv.Atoi(portStr)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := telegram.NewClient(1, "hash", telegram.Options{
		PublicKeys: []exchange.PublicKey{{RSA: &priv.PublicKey}},
		Resolver:   resolver{host: host, port: port},
		DCList:     dcs.List{Options: []tg.DCOption{{ID: 2, IPAddress: host, Port: port, Static: true}}},
		Logger:     logzap.New(logger.Named("client")),
	})
	err = client.Run(ctx, func(ctx context.Context) error {
		api := tg.NewClient(client)
		res, err := api.AuthSendCode(ctx, &tg.AuthSendCodeRequest{
			PhoneNumber: *phone,
			APIID:       1,
			APIHash:     "hash",
			Settings:    tg.CodeSettings{},
		})
		if err != nil {
			fmt.Printf("sendCode error: %v\n", err)
			return nil
		}
		switch v := res.(type) {
		case *tg.AuthSentCode:
			fmt.Printf("sent code hash=%s type=%T\n", v.PhoneCodeHash, v.Type)
		default:
			fmt.Printf("sent code %T\n", res)
		}
		return nil
	})
	if err != nil {
		fmt.Println("run:", err)
	}
}
