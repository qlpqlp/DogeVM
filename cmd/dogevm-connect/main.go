// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/DogecoinVM/dogevm-connect/internal/config"
	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
	"github.com/DogecoinVM/dogevm-connect/internal/server"
	"github.com/DogecoinVM/dogevm-connect/internal/tracker"
	"github.com/DogecoinVM/dogevm-connect/internal/wallet"
	"github.com/DogecoinVM/dogevm-connect/web"
)

// releaseVersion is set by release builds with -X main.releaseVersion=v1.0.0
var releaseVersion = "dev"

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "HTTP listen address")
	base := flag.String("api", metalapi.DefaultBaseURL, "DogecoinVM API base URL")
	dataDir := flag.String("data", "", "wallet data directory (default: OS user config)")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser")
	userToken := flag.String("user-token", "", "user API token (overrides config file and DOGEVM_USER_TOKEN)")
	adminToken := flag.String("admin-token", "", "admin API token (overrides config file and DOGEVM_ADMIN_TOKEN)")
	flag.Parse()

	log.Printf("version %s", releaseVersion)

	dir := *dataDir
	if dir == "" {
		dir = defaultDataDir()
	}

	store, err := wallet.NewStore(dir)
	if err != nil {
		log.Fatal(err)
	}
	access, err := config.Load(dir)
	if err != nil {
		log.Fatal(err)
	}
	if *userToken != "" || *adminToken != "" {
		user, admin := access.Tokens()
		if *userToken != "" {
			user = *userToken
		}
		if *adminToken != "" {
			admin = *adminToken
		}
		if err := access.SetMemory(user, admin); err != nil {
			log.Fatal(err)
		}
	}
	if access.Required() {
		log.Printf("API tokens: on")
	} else {
		log.Printf("API tokens: off (local open access)")
	}
	client := metalapi.New(*base)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Pin network before serving signed actions.
	info, err := client.Info(ctx)
	if err != nil {
		log.Printf("warning: could not reach %s yet: %v", *base, err)
	} else if info.ChainID != wallet.ExpectedChainID || info.DogecoinVMNetwork != wallet.ExpectedDogecoinVMNet {
		log.Fatalf("unexpected network chainID=%s net=%s", info.ChainID, info.DogecoinVMNetwork)
	} else {
		log.Printf("connected to DogecoinVM chainID=%s peg=%s", info.ChainID, info.PegAddress)
	}

	tr := tracker.New(client)
	tr.Start(ctx)

	srv := &server.Server{
		Store:   store,
		Client:  client,
		Tracker: tr,
		Static:  web.FS(),
		Info:    info,
		Access:  access,
	}

	go func() {
		if err := server.ListenAndServe(*addr, srv); err != nil {
			log.Fatal(err)
		}
	}()

	url := "http://" + *addr + "/"
	log.Printf("wallet data: %s", dir)
	log.Printf("open %s", url)
	if !*noBrowser {
		time.Sleep(300 * time.Millisecond)
		_ = openBrowser(url)
	}

	<-ctx.Done()
	log.Println("shutting down")
}

func defaultDataDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("AppData")
		if base == "" {
			base = "."
		}
		return filepath.Join(base, "DogeVMConnect", "data")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "./dogevm-connect-data"
	}
	return filepath.Join(home, ".dogevm-connect", "data")
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("dogevm-connect: ")
}
