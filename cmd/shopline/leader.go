package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// runOutboxLeader campaigns for /shopline/outbox-relay. Only the winner
// runs onLead until the etcd session ends, then campaigns again.
func runOutboxLeader(ctx context.Context, onLead func(context.Context)) {
	endpoints := os.Getenv("ETCD_ENDPOINTS")
	if endpoints == "" {
		endpoints = "localhost:2379"
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(endpoints, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatalf("etcd: %v", err)
	}

	go func() {
		defer cli.Close()
		host, _ := os.Hostname()
		for {
			if ctx.Err() != nil {
				return
			}
			session, err := concurrency.NewSession(cli, concurrency.WithTTL(10))
			if err != nil {
				log.Printf("etcd session: %v", err)
				time.Sleep(time.Second)
				continue
			}
			elec := concurrency.NewElection(session, "/shopline/outbox-relay")
			if err := elec.Campaign(ctx, host); err != nil {
				session.Close()
				if ctx.Err() != nil {
					return
				}
				log.Printf("etcd campaign: %v", err)
				continue
			}
			log.Printf("outbox relay leader: %s", host)
			leadCtx, cancel := context.WithCancel(ctx)
			go func() {
				<-session.Done()
				cancel()
			}()
			onLead(leadCtx)
			cancel()
			session.Close()
			log.Printf("outbox relay leader lost: %s", host)
		}
	}()
}
