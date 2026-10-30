package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/grpc"
)

type daemon struct {
	proto.UnimplementedDaemonServiceServer
}

func (daemon) GetFeatures(context.Context, *proto.GetFeaturesRequest) (*proto.GetFeaturesResponse, error) {
	return &proto.GetFeaturesResponse{DisableProfiles: true}, nil
}
func (daemon) GetConfig(context.Context, *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	return &proto.GetConfigResponse{ManagementUrl: "https://aheadscale.ru", AdminURL: "https://aheadscale.ru"}, nil
}
func (daemon) Login(context.Context, *proto.LoginRequest) (*proto.LoginResponse, error) {
	return &proto.LoginResponse{NeedsSSOLogin: true, UserCode: "TEST-CODE"}, nil
}
func (daemon) WaitSSOLogin(context.Context, *proto.WaitSSOLoginRequest) (*proto.WaitSSOLoginResponse, error) {
	return &proto.WaitSSOLoginResponse{Email: "test@example.invalid"}, nil
}
func (daemon) Up(context.Context, *proto.UpRequest) (*proto.UpResponse, error) {
	return &proto.UpResponse{}, nil
}
func (daemon) Down(context.Context, *proto.DownRequest) (*proto.DownResponse, error) {
	return &proto.DownResponse{}, nil
}
func (daemon) Status(context.Context, *proto.StatusRequest) (*proto.StatusResponse, error) {
	return &proto.StatusResponse{Status: "Connected"}, nil
}

func main() {
	socket := flag.String("socket", "", "Закрытый Unix socket protocol-fixture")
	flag.Parse()
	if *socket == "" {
		log.Fatal("socket обязателен")
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	defer func() {
		if err := os.Remove(*socket); err != nil && !os.IsNotExist(err) {
			log.Print(err)
		}
	}()
	if err := os.Chmod(*socket, 0600); err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := grpc.NewServer()
	proto.RegisterDaemonServiceServer(server, daemon{})
	go func() { <-ctx.Done(); server.Stop() }()
	if err := server.Serve(listener); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
