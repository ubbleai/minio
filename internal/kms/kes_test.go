// Copyright (c) 2015-2022 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package kms

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/kes"
)

// TestDecryptAllWithPendingReload checks that DecryptAll does not deadlock when
// the KES client certificate is reloaded (write lock) while it is decrypting.
// sync.RWMutex blocks new readers once a writer is waiting, so DecryptAll must
// not take the read lock a second time for each ciphertext.
func TestDecryptAllWithPendingReload(t *testing.T) {
	firstCall, release := make(chan struct{}), make(chan struct{})
	var calls int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(firstCall)
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"plaintext":"cGxhaW50ZXh0"}`))
	}))
	defer server.Close()

	client := kes.NewClientWithConfig("", &tls.Config{InsecureSkipVerify: true})
	client.Endpoints = []string{server.URL}
	c := &kesClient{client: client}

	done := make(chan error, 1)
	go func() {
		_, err := c.DecryptAll(context.Background(), "key",
			[][]byte{[]byte("ciphertext-1"), []byte("ciphertext-2")},
			[]Context{{"bucket": "bucket/object-1"}, {"bucket": "bucket/object-2"}})
		done <- err
	}()

	<-firstCall
	reloaded := make(chan struct{})
	go func() { // what the certificate reload goroutine of NewWithConfig does
		c.lock.Lock()
		c.lock.Unlock()
		close(reloaded)
	}()
	time.Sleep(100 * time.Millisecond) // let the writer start waiting
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DecryptAll: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DecryptAll deadlocked against a pending write lock")
	}
	select {
	case <-reloaded:
	case <-time.After(5 * time.Second):
		t.Fatal("the write lock was never acquired")
	}
}
