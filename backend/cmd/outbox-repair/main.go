// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Command outbox-repair requeues one exact dead-letter marker for normal relay delivery.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/infrastructure/db"
)

func main() {
	outbox := flag.String("outbox", "", "dead-letter outbox to repair: fga or index")
	objectType := flag.String("object-type", "", "exact object type")
	objectUID := flag.String("object-uid", "", "exact object UID")
	relation := flag.String("relation", "", "exact FGA membership relation")
	username := flag.String("username", "", "exact FGA membership username")
	flag.Parse()

	if *outbox == "" || *objectType == "" || *objectUID == "" {
		log.Fatal("--outbox, --object-type, and --object-uid are required")
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, db.PoolConfig{MaxConns: 2})
	if err != nil {
		log.Fatalf("database pool: %v", err)
	}
	defer pool.Close()

	var requeued bool
	switch *outbox {
	case "fga":
		if (*relation == "") != (*username == "") {
			log.Fatal("--relation and --username must be provided together")
		}
		requeued, err = db.NewFGAOutboxRepository(pool).RequeueDeadLetter(ctx, *objectType, *objectUID, *relation, *username)
	case "index":
		if *relation != "" || *username != "" {
			log.Fatal("--relation and --username are only valid for the FGA outbox")
		}
		requeued, err = db.NewIndexOutboxRepository(pool).RequeueDeadLetter(ctx, *objectType, *objectUID)
	default:
		log.Fatalf("unsupported --outbox %q: use fga or index", *outbox)
	}
	if err != nil {
		log.Fatal(err)
	}
	if !requeued {
		log.Fatalf("no matching dead-letter marker for %s:%s in %s outbox", *objectType, *objectUID, *outbox)
	}

	fmt.Printf("requeued %s dead-letter marker for %s:%s\n", *outbox, *objectType, *objectUID)
}
