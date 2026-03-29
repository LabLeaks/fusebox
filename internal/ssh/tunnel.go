package ssh

import (
	"fmt"
	"io"
	"net"
	"sync"
)

// ReverseTunnel represents an active SSH reverse tunnel (-R).
// Remote connections to remotePort on the server are forwarded to localPort on the client.
type ReverseTunnel struct {
	listener net.Listener
	localPort int
	done     chan struct{}
	wg       sync.WaitGroup
}

// Close tears down the reverse tunnel.
func (t *ReverseTunnel) Close() error {
	close(t.done)
	err := t.listener.Close()
	t.wg.Wait()
	return err
}

// ReverseTunnel establishes an SSH reverse tunnel: connections to remotePort on the
// server are forwarded to localPort on the local machine.
func (c *Client) ReverseTunnel(remotePort, localPort int) (io.Closer, error) {
	remoteAddr := fmt.Sprintf("127.0.0.1:%d", remotePort)
	listener, err := c.client.Listen("tcp", remoteAddr)
	if err != nil {
		return nil, fmt.Errorf("listening on remote %s: %w", remoteAddr, err)
	}

	tunnel := &ReverseTunnel{
		listener:  listener,
		localPort: localPort,
		done:      make(chan struct{}),
	}

	tunnel.wg.Add(1)
	go tunnel.accept()

	return tunnel, nil
}

func (t *ReverseTunnel) accept() {
	defer t.wg.Done()

	for {
		remote, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.done:
				return
			default:
				continue
			}
		}

		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.forward(remote)
		}()
	}
}

func (t *ReverseTunnel) forward(remote net.Conn) {
	defer remote.Close()

	localAddr := fmt.Sprintf("127.0.0.1:%d", t.localPort)
	local, err := net.Dial("tcp", localAddr)
	if err != nil {
		return
	}
	defer local.Close()

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(local, remote)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(remote, local)
		done <- struct{}{}
	}()

	// Wait for one direction to finish, then close both
	<-done
}
