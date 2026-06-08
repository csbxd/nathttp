package nathttp

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/csbxd/natnet"
)

const (
	stunBindingResponse = 0x0101
	stunHeaderSize      = 20
	stunTxIDSize        = 12
	stunCookie          = 0x2112a442
	stunXORMappedAddr   = 0x0020
	stunIPv4            = 0x01
)

func TestServeStartsRunnerAndStopsOnContext(t *testing.T) {
	stunLn, stunAddr := startSTUNServer(t, netip.MustParseAddrPort("203.0.113.11:26656"))
	defer stunLn.Close()

	httpLn, keepAliveAddr := startKeepAliveServer(t)
	defer httpLn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan natnet.Addr, 1)

	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, &http.Server{
			Addr: "127.0.0.1:0",
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}),
		}, Config{
			NAT: natnet.Config{
				STUNServers:       []string{stunAddr},
				KeepAliveHosts:    []string{keepAliveAddr},
				Timeout:           200 * time.Millisecond,
				ProbeInterval:     10 * time.Millisecond,
				KeepAliveInterval: 10 * time.Millisecond,
				MaxErrors:         1,
				Syncer: natnet.SyncFunc(func(ctx context.Context, addr natnet.Addr) error {
					select {
					case got <- addr:
					default:
					}
					return nil
				}),
			},
		})
	}()

	select {
	case addr := <-got:
		if addr.String() != "203.0.113.11:26656" {
			t.Fatalf("got %s", addr)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sync")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("Serve returned %v; want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Serve to return")
	}
}

func startSTUNServer(t *testing.T, public natnet.Addr) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSTUN(conn, public)
		}
	}()
	return ln, ln.Addr().String()
}

func serveSTUN(conn net.Conn, public natnet.Addr) {
	defer conn.Close()
	for {
		var req [stunHeaderSize]byte
		if _, err := io.ReadFull(conn, req[:]); err != nil {
			return
		}
		var txid [stunTxIDSize]byte
		copy(txid[:], req[8:20])
		attr := makeXORAddressAttr(public)
		var hdr [stunHeaderSize]byte
		binary.BigEndian.PutUint16(hdr[0:2], stunBindingResponse)
		binary.BigEndian.PutUint16(hdr[2:4], uint16(len(attr)))
		binary.BigEndian.PutUint32(hdr[4:8], stunCookie)
		copy(hdr[8:20], txid[:])
		if _, err := conn.Write(hdr[:]); err != nil {
			return
		}
		if _, err := conn.Write(attr); err != nil {
			return
		}
	}
}

func makeXORAddressAttr(addr natnet.Addr) []byte {
	var body [12]byte
	ip := addr.Addr().As4()
	binary.BigEndian.PutUint16(body[0:2], stunXORMappedAddr)
	binary.BigEndian.PutUint16(body[2:4], 8)
	body[5] = stunIPv4
	binary.BigEndian.PutUint16(body[6:8], addr.Port()^uint16(stunCookie>>16))
	body[8] = ip[0] ^ byte((stunCookie>>24)&0xff)
	body[9] = ip[1] ^ byte((stunCookie>>16)&0xff)
	body[10] = ip[2] ^ byte((stunCookie>>8)&0xff)
	body[11] = ip[3] ^ byte(stunCookie&0xff)
	return body[:]
}

func startKeepAliveServer(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveKeepAlive(conn)
		}
	}()
	return ln, ln.Addr().String()
}

func serveKeepAlive(conn net.Conn) {
	defer conn.Close()
	var buf [512]byte
	for {
		if _, err := conn.Read(buf[:]); err != nil {
			return
		}
		if _, err := conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: keep-alive\r\n\r\n")); err != nil {
			return
		}
	}
}
