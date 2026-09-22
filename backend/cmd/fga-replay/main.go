// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command fga-replay returns a dead-lettered FGA marker to the relay queue.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: fga-replay <outbox-marker-id>")
	}
	id, err := strconv.ParseInt(os.Args[1], 10, 64)
	if err != nil || id <= 0 {
		log.Fatalf("invalid outbox marker ID %q", os.Args[1])
	}
	pool, err := db.NewPool(context.Background(), db.PoolConfig{MaxConns: 2, MinConns: 1})
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.NewFGAOutboxRepository(pool).ReplayDeadLetter(context.Background(), id); err != nil {
		log.Fatal(fmt.Errorf("replay marker: %w", err))
	}
	fmt.Printf("replayed dead-letter marker %d\n", id)
}
