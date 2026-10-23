// Command smoke verifies the final adapter image against isolated broker and HTTP fixtures.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const goodActor = "11111111-1111-4111-8111-111111111111"
const badActor = "22222222-2222-4222-8222-222222222222"

type request struct {
	Method string
	Path   string
	Query  string
	Body   map[string]interface{}
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("expected serve or verify")
	}
	if os.Args[1] == "serve" {
		serve()
		return
	}
	if os.Args[1] != "verify" {
		log.Fatal("unknown mode")
	}
	if err := verify(); err != nil {
		log.Fatal(err)
	}
}

func serve() {
	var mu sync.Mutex
	var requests []request
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/requests" {
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(requests)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-as-token" {
			http.Error(w, `{"errcode":"M_FORBIDDEN"}`, http.StatusForbidden)
			return
		}
		body := map[string]interface{}{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		mu.Lock()
		requests = append(requests, request{r.Method, r.URL.Path, r.URL.RawQuery, body})
		mu.Unlock()
		var response interface{} = map[string]interface{}{}
		switch {
		case r.URL.Path == "/_matrix/client/v3/register":
			if body["username"] == badActor {
				w.WriteHeader(http.StatusForbidden)
				response = map[string]interface{}{"errcode": "M_FORBIDDEN", "error": "fixture actor rejected"}
			} else {
				response = map[string]interface{}{"user_id": fmt.Sprint("@", body["username"], ":fixture.invalid"), "access_token": "fixture-as-token"}
			}
		case strings.HasSuffix(r.URL.Path, "/account/whoami"):
			response = map[string]interface{}{"user_id": "@00000000-0000-0000-0000-000000000000:fixture.invalid"}
		case strings.HasPrefix(r.URL.Path, "/_synapse/admin/v2/users/"):
			response = map[string]interface{}{"admin": true, "name": "@00000000-0000-0000-0000-000000000000:fixture.invalid"}
		case strings.HasSuffix(r.URL.Path, "/displayname") && r.Method == "PUT":
		default:
			w.WriteHeader(http.StatusNotFound)
			response = map[string]interface{}{"errcode": "M_UNRECOGNIZED", "error": "unexpected fixture route"}
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	log.Fatal((&http.Server{Addr: ":8081", Handler: mux, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}

func verify() error {
	var conn *amqp.Connection
	var err error
	for i := 0; i < 60; i++ {
		conn, err = amqp.Dial("amqp://fixture:fixture@rabbit:5672/")
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	const topic = "communication.actor.sync"
	ready := false
	for i := 0; i < 60; i++ {
		// A passive declaration on a separate channel avoids closing the RPC channel during startup.
		probe, e := conn.Channel()
		if e != nil {
			return e
		}
		q, e := probe.QueueDeclarePassive(topic, true, false, false, false, nil)
		_ = probe.Close()
		if e == nil && q.Consumers > 0 {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}
	if !ready {
		return fmt.Errorf("adapter never subscribed to actor command")
	}
	reply, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return err
	}
	deliveries, err := ch.Consume(reply.Name, "", true, true, false, false, nil)
	if err != nil {
		return err
	}
	for _, actor := range []string{goodActor, badActor} {
		payload, _ := json.Marshal(map[string]string{"actor_id": actor, "display_name": "Fixture Actor"})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = ch.PublishWithContext(ctx, "", topic, true, false, amqp.Publishing{ContentType: "application/json", MessageId: actor, CorrelationId: actor, ReplyTo: reply.Name, Body: payload})
		if err != nil {
			cancel()
			return err
		}
		select {
		case delivery := <-deliveries:
			cancel()
			if delivery.CorrelationId != actor {
				return fmt.Errorf("lost RPC correlation")
			}
			var response struct {
				Success bool `json:"success"`
				Error   *struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err = json.Unmarshal(delivery.Body, &response); err != nil {
				return err
			}
			if actor == goodActor && (!response.Success || response.Error != nil) {
				return fmt.Errorf("success RPC failed: %s", delivery.Body)
			}
			if actor == badActor && (response.Success || response.Error == nil || response.Error.Code != "NOT_ALLOWED" || !strings.Contains(response.Error.Message, "fixture actor rejected")) {
				return fmt.Errorf("forbidden Matrix error not surfaced: %s", delivery.Body)
			}
			fmt.Printf("RPC %s: %s\n", actor, delivery.Body)
		case <-ctx.Done():
			cancel()
			return fmt.Errorf("RPC reply missing for %s", actor)
		}
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://matrix:8081/requests")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var requests []request
	if err = json.NewDecoder(resp.Body).Decode(&requests); err != nil {
		return err
	}
	profile, badRegistration := false, false
	for _, r := range requests {
		if r.Method == "PUT" && strings.Contains(r.Path, goodActor) && strings.HasSuffix(r.Path, "/displayname") && r.Body["displayname"] == "Fixture Actor" {
			profile = true
		}
		if r.Method == "POST" && r.Path == "/_matrix/client/v3/register" && r.Body["username"] == badActor {
			badRegistration = true
		}
	}
	if !profile || !badRegistration {
		return fmt.Errorf("expected Matrix requests missing: %+v", requests)
	}
	q, err := ch.QueueInspect(topic)
	if err != nil {
		return err
	}
	if q.Messages != 0 {
		return fmt.Errorf("commands not acknowledged")
	}
	fmt.Println("Matrix profile request and forbidden registration verified; broker queue drained")
	return nil
}
