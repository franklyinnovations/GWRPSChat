package core

import (
	"time"
	"fmt"
	"github.com/garyburd/redigo/redis"
)

//死活監視間隔
const HealthCheckPeriod = time.Minute

type RedisConnect struct {
	SubConn *redis.PubSubConn
	PubConn *redis.Conn
	channel string
	Done chan error
	//var done = make(chan error, 1)
}

func (r *RedisConnect)InitConn(address string) error{
	var err error
	var tmpConn redis.Conn 
	tmpConn, err = redis.Dial("tcp", address,
		// Read timeout on server should be greater than ping period.
		redis.DialReadTimeout(HealthCheckPeriod+10*time.Second),
		redis.DialWriteTimeout(10*time.Second))
	if err != nil {
		return err
	}
	r.SubConn = &redis.PubSubConn{Conn: tmpConn}

	tmpConn, err = redis.Dial("tcp", address)
	if err != nil {
		fmt.Println(err)
		return err
	}
	r.PubConn = &tmpConn
	return nil
}

func (r *RedisConnect)CloseConn(){
	r.SubConn.Unsubscribe()
	r.SubConn.Close()
	(*r.PubConn).Close()
}

func (r *RedisConnect)Publish(channel string,message []byte) {
	(*r.PubConn).Do("PUBLISH", channel, message)
}

// This example shows how receive pubsub notifications with cancelation and
// health checks.
func (r *RedisConnect)Subscribe(channel string,onMessage func(channel string, data []byte) error) {

	err := r.listenPubSubChannels(onMessage,channel)

	if err != nil {
		fmt.Println(err)
		return
	}
}

// L listens for messages on Redis pubsub channels. The
// onStart function is called after the channels are subscribed. The onMessage
// function is called for each message.
func (r *RedisConnect)listenPubSubChannels(
	onMessage func(channel string, data []byte) error,
	channels ...string) error {
	// A ping is set to the server with this period to test for the health of
	// the connection and server.
	if err := r.SubConn.Subscribe(redis.Args{}.AddFlat(channels)...); err != nil {
		return err
	}
    fmt.Println("Subscribe Success ")

	// Start a goroutine to receive notifications from the server.
	go func() {
		for {
			switch n := r.SubConn.Receive().(type) {
			case error:
				r.Done <- n
				return
			case redis.Message:
				if err := onMessage(n.Channel, n.Data); err != nil {
					r.Done <- err
					return
				}
			case redis.Subscription:
				fmt.Println("Subscription Count Change:",n.Count) 
			}
		}
	}()

	//死活監視
	go func() error{
		ticker := time.NewTicker(HealthCheckPeriod)
		defer ticker.Stop()
		defer r.CloseConn()
		for {
			select {
			case <-ticker.C:
				// Send ping to test health of connection and server. If
				// corresponding pong is not received, then receive on the
				// connection will timeout and the receive goroutine will exit.
				if err := r.SubConn.Ping(""); err != nil {
					return err 
				}
			case err := <-r.Done:
				// Return error from the receive goroutine.
				return err
			}
		}
	}()

	// Wait for goroutine to complete.
	return nil
}

