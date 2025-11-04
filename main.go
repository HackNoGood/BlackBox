package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"io/ioutil"

	"github.com/HackNoGood/BlackBox/internal/ui"
	libp2p "github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	tcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/multiformats/go-multiaddr"
)

const (
	topicName = "blackbox/lobby"

	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[92m"
	ansiCyan   = "\x1b[96m"
	ansiYellow = "\x1b[93m"
	ansiBlue   = "\x1b[94m"
	ansiDim    = "\x1b[2m"
)

// Ensure checks if a key file exists; if not, it generates and saves one.
func Ensure(path string) (crypto.PrivKey, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		priv, _, err := crypto.GenerateEd25519Key(nil)
		if err != nil {
			return nil, err
		}
		data, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return nil, err
		}
		return priv, nil
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	priv, err := crypto.UnmarshalPrivateKey(data)
	if err != nil {
		return nil, err
	}
	return priv, nil
}

// waitForRelayAndPrint polls until a /p2p-circuit address appears and prints it.
func waitForRelayAndPrint(h host.Host, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	printed := false

	for time.Now().Before(deadline) && !printed {
		for _, a := range h.Addrs() {
			if strings.Contains(a.String(), "/p2p-circuit") {
				ra := a.Encapsulate(multiaddr.StringCast("/p2p/" + h.ID().String()))
				fmt.Println("→ Relay (share this over the internet, no port-forwarding needed):")
				fmt.Printf("   %s\n", ra.String())
				printed = true
				break
			}
		}
		if !printed {
			time.Sleep(750 * time.Millisecond)
		}
	}

	if !printed {
		fmt.Println("(No relay address yet. If you’re on LAN, use the 192.168.x.x line; otherwise add --relays or port-forward.)")
	}
}

// parse a comma-separated list of relay multiaddrs into AddrInfos
func parseRelayInfos(csv string) ([]peer.AddrInfo, error) {
	if strings.TrimSpace(csv) == "" {
		return nil, nil
	}
	parts := strings.Split(csv, ",")
	infos := make([]peer.AddrInfo, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		maddr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			return nil, fmt.Errorf("bad relay multiaddr %q: %w", s, err)
		}
		ai, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			return nil, fmt.Errorf("cannot parse relay AddrInfo %q: %w", s, err)
		}
		infos = append(infos, *ai)
	}
	return infos, nil
}

func main() {
	// 🟢 Boot animation + menu selection
	mode := ui.MainMenu()

	var (
		joinAddr string
		username string
		hostMode bool
	)

	switch mode {
	case "host":
		hostMode = true
	case "join":
		fmt.Print("Enter connection address: ")
		fmt.Scanln(&joinAddr)
	default:
		fmt.Println("Invalid selection. Exiting.")
		return
	}

	fmt.Print("Enter your username: ")
	fmt.Scanln(&username)

	// flags
	port := flag.Int("port", 4001, "Listen port for libp2p (use a different port to run multiple local instances)")
	relaysCSV := flag.String("relays", "", "Comma-separated relay multiaddrs (to enable AutoRelay)")
	flag.Parse()

	// Identity setup
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".blackbox")
	keyFileName := fmt.Sprintf("node_%d.key", *port)
	keyPath := filepath.Join(dataDir, keyFileName)
	_ = os.MkdirAll(dataDir, 0700)

	priv, err := Ensure(keyPath)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listenAddrs := []string{
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port),
		fmt.Sprintf("/ip6/::/tcp/%d", *port),
	}

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.ListenAddrStrings(listenAddrs...),
		// NAT helpers are safe regardless
		libp2p.NATPortMap(),
	}

	// If user supplied relays, enable AutoRelay using those static relays
	if *relaysCSV != "" {
		staticRelays, err := parseRelayInfos(*relaysCSV)
		if err != nil {
			log.Fatalf("parse --relays: %v", err)
		}
		if len(staticRelays) == 0 {
			log.Println("[warn] --relays provided but none parsed; continuing without AutoRelay")
		} else {
			opts = append(opts,
				libp2p.EnableAutoRelayWithStaticRelays(staticRelays, nil),
				libp2p.EnableHolePunching(),
			)
			fmt.Println("[AutoRelay] Using static relays from --relays")
		}
	} else {
		fmt.Println("[AutoRelay] No relays provided. Running without AutoRelay (no panic).")
	}

	h, err := libp2p.New(opts...)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		log.Fatal(err)
	}
	t, err := ps.Join(topicName)
	if err != nil {
		log.Fatal(err)
	}
	sub, err := t.Subscribe()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("You are known as: %s%s%s\n", ansiCyan, username, ansiReset)

	if hostMode {
		fmt.Println("\nStarting BlackBox host...")
		fmt.Println("Your connection address(es):")

		// Show immediate addrs (loopback/LAN/etc)
		printHostInfo(h)

		// If AutoRelay is active, print the relay addr when available
		if *relaysCSV != "" {
			waitForRelayAndPrint(h, 20*time.Second)
		} else {
			fmt.Println("(Tip) To get a relay address (no port-forward), restart with:")
			fmt.Println("      --relays <relay-multiaddr[,relay2,...]>")
		}

	} else if joinAddr != "" {
		fmt.Println("\nAttempting to join BlackBox chat...")
		maddr, err := multiaddr.NewMultiaddr(joinAddr)
		if err != nil {
			log.Fatalf("invalid multiaddr: %v\n", err)
		}
		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Fatalf("failed to parse peer AddrInfo: %v\n", err)
		}
		if info.ID == h.ID() {
			fmt.Println("[Info] That address points to your own node — already hosting.")
			return
		}
		if !addrInfoIsReachable(info, 1500*time.Millisecond) {
			fmt.Println("\n[Notice] Host may be behind firewall — still attempting...")
		}
		if err := connectToPeer(ctx, h, joinAddr); err != nil {
			log.Fatalf("connect failed: %v\n", err)
		}

		if runtime.GOOS != "windows" {
			fmt.Print("\033[H\033[2J")
			logoPath := filepath.Join("assets", "blackboxlogo.sh")
			cmd := exec.Command("bash", logoPath)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
		}

		fmt.Println("\n──────────────────────────────────────────")
		fmt.Printf("%sWelcome to BlackBox Chat!%s\n", ansiGreen, ansiReset)
		fmt.Printf("%sNode:%s Client\n", ansiCyan, ansiReset)
		fmt.Printf("%sYour ID:%s %s\n", ansiCyan, ansiReset, h.ID().String())
		fmt.Printf("%sConnected to:%s %s\n", ansiCyan, ansiReset, joinAddr)
		fmt.Println("──────────────────────────────────────────")
		fmt.Printf("%sType /help to see available commands%s\n", ansiDim, ansiReset)
		fmt.Println("──────────────────────────────────────────")
	}

	// Message listener
	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				return
			}
			if msg.ReceivedFrom == h.ID() {
				continue
			}
			msgText := string(msg.Data)
			parts := strings.SplitN(msgText, "]:", 2)
			if len(parts) == 2 && strings.HasPrefix(parts[0], "[") {
				username := strings.TrimPrefix(parts[0], "[")
				message := parts[1]
				fmt.Printf("\n%s%s%s: %s\n> ", ansiYellow, username, ansiReset, message)
			} else {
				fmt.Printf("\n%s\n> ", msgText)
			}
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Print("> ")
			continue
		}

		switch {
		case line == "/help":
			printHelp()
			fmt.Print("> ")
			continue
		case line == "/exit":
			fmt.Println("\n[BlackBox] Disconnecting...")
			cancel()
			os.Exit(0)
		}

		message := fmt.Sprintf("[%s]:%s", username, line)
		if err := t.Publish(ctx, []byte(message)); err != nil {
			fmt.Println("publish error:", err)
		}
		fmt.Printf("%s%s%s: %s\n> ", ansiBlue, username, ansiReset, line)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("\n[BlackBox] shutting down cleanly...")
}

// 👇 printHelp now lives outside main()
func printHelp() {
	fmt.Print(`
BlackBox v0.1.0 — Secure Peer-to-Peer Chat

Quick Start:
  1. To host a chat:
     ./blackbox --host --username YourName

  2. To join a chat:
     ./blackbox --join <connection-address> --username YourName

Usage:
  ./blackbox [--host] [--join <multiaddr>] [--username <n>] [--port <n>] [--relays <maddr[,maddr...] ] 

Options:
  --host               Start a new BlackBox node and host a lobby
  --join <multiaddr>   Join an existing peer by address
  --username <name>    Set your chat username
  --port <n>           Listen port (default 4001)
  --relays <list>      Comma-separated relay multiaddrs to enable AutoRelay
  --help               Show this help message

Notes:
 - When joining, the CLI validates the multiaddr and checks reachability before attempting a libp2p connect.
 - To avoid manual port-forwarding, host with --relays and share the printed /p2p-circuit address.
`)
}

// connectToPeer tries to perform a libp2p connect and returns an error if it fails.
func connectToPeer(ctx context.Context, h host.Host, addr string) error {
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return fmt.Errorf("bad AddrInfo: %w", err)
	}
	if err := h.Connect(ctx, *info); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	fmt.Println("\n✓ Successfully connected to the chat!")
	fmt.Println("→ You can start typing messages now.")
	fmt.Println("→ Type /help to see available commands.")
	return nil
}

// printHostInfo prints your host node's connection address info.
func printHostInfo(h host.Host) {
	var loopback, lan, others []string
	for _, a := range h.Addrs() {
		withID := a.Encapsulate(multiaddr.StringCast("/p2p/" + h.ID().String())).String()
		s := a.String()
		switch {
		case strings.Contains(s, "/ip4/127.0.0.1") || strings.Contains(s, "/ip6/::1"):
			loopback = append(loopback, withID)
		case strings.Contains(s, "/ip4/10.") || strings.Contains(s, "/ip4/192.168.") || strings.Contains(s, "/ip4/172."):
			lan = append(lan, withID)
		default:
			others = append(others, withID) // may include public or relay if already present
		}
	}

	fmt.Println("──────────────────────────────────────────")
	fmt.Printf("Node ID: %s\n", h.ID().String())
	if len(others) > 0 {
		fmt.Println("→ Other:")
		for _, a := range others {
			fmt.Printf("   %s\n", a)
		}
	}
	if len(lan) > 0 {
		fmt.Println("→ LAN (same Wi-Fi/router only):")
		for _, a := range lan {
			fmt.Printf("   %s\n", a)
		}
	}
	if len(loopback) > 0 {
		fmt.Println("→ Loopback (this machine only):")
		for _, a := range loopback {
			fmt.Printf("   %s\n", a)
		}
	}
	fmt.Println("──────────────────────────────────────────")
}

// addrInfoIsReachable checks if a peer address is reachable via TCP.
func addrInfoIsReachable(info *peer.AddrInfo, timeout time.Duration) bool {
	for _, a := range info.Addrs {
		s := a.String()
		var ip, port string
		_, _ = fmt.Sscanf(s, "/ip4/%[^/]/tcp/%[^/]/p2p/%*s", &ip, &port)
		if ip != "" && port != "" {
			addr := net.JoinHostPort(ip, port)
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err == nil {
				conn.Close()
				return true
			}
		}
		_, _ = fmt.Sscanf(s, "/dns/%[^/]/tcp/%[^/]/p2p/%*s", &ip, &port)
		if ip != "" && port != "" {
			addr := net.JoinHostPort(ip, port)
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err == nil {
				conn.Close()
				return true
			}
		}
	}
	return false
}
