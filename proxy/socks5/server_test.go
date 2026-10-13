package socks5

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/nadoo/glider/pkg/socks"
)

func TestHandshakeSelectsOfferedMethod(t *testing.T) {
	for _, tt := range []struct {
		name     string
		user     string
		password string
		methods  []byte
		want     byte
	}{
		{name: "no authentication", methods: []byte{socks.AuthNone}, want: socks.AuthNone},
		{name: "password authentication", user: "user", password: "pass", methods: []byte{socks.AuthPassword}, want: socks.AuthPassword},
		{name: "password offered second", user: "user", password: "pass", methods: []byte{socks.AuthNone, socks.AuthPassword}, want: socks.AuthPassword},
		{name: "no supported method", methods: []byte{0x80}, want: 0xff},
		{name: "no methods", want: 0xff},
		{name: "password not offered", user: "user", password: "pass", methods: []byte{socks.AuthNone}, want: 0xff},
		{name: "no-auth not offered", methods: []byte{socks.AuthPassword}, want: 0xff},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { client.Close(); server.Close() })
			client.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan error, 1)
			go func() {
				s := &Socks5{user: tt.user, password: tt.password}
				_, err := s.handshake(server)
				server.Close()
				done <- err
			}()
			greeting := append([]byte{Version, byte(len(tt.methods))}, tt.methods...)
			if _, err := client.Write(greeting); err != nil {
				t.Fatal(err)
			}
			var reply [2]byte
			if _, err := io.ReadFull(client, reply[:]); err != nil {
				t.Fatal(err)
			}
			if reply != [2]byte{Version, tt.want} {
				t.Errorf("method reply = %v, want [%d %d]", reply, Version, tt.want)
			}
			client.Close()
			if err := <-done; err == nil {
				t.Error("incomplete handshake unexpectedly succeeded")
			}
		})
	}
}
