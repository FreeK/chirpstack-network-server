package asclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/brocaar/loraserver/api/as"
)

// Pool defines the application-server client pool.
type Pool interface {
	Get(hostname string, caCert, tlsCert, tlsKey []byte) (as.ApplicationServerClient, error)
}

type client struct {
	lastUsed time.Time
	client   as.ApplicationServerClient
}

type pool struct {
	sync.RWMutex
	clients map[string]client
}

// NewPool creates a new Pool.
func NewPool() Pool {
	return &pool{
		clients: make(map[string]client),
	}
}

// Get Returns an ApplicationServerClient for the given server (hostname:ip).
func (p *pool) Get(hostname string, caCert, tlsCert, tlsKey []byte) (as.ApplicationServerClient, error) {
	defer p.Unlock()
	p.Lock()

	c, ok := p.clients[hostname]
	if !ok {
		asClient, err := p.createClient(hostname, caCert, tlsCert, tlsKey)
		if err != nil {
			return nil, errors.Wrap(err, "create application-server api client error")
		}
		c = client{
			lastUsed: time.Now(),
			client:   asClient,
		}
		p.clients[hostname] = c
	}

	return c.client, nil
}

func (p *pool) createClient(hostname string, caCert, tlsCert, tlsKey []byte) (as.ApplicationServerClient, error) {
	asOpts := []grpc.DialOption{
		grpc.WithBlock(),
	}

	if len(tlsCert) == 0 && len(tlsKey) == 0 && len(caCert) == 0 {
		asOpts = append(asOpts, grpc.WithInsecure())
		log.WithField("server", hostname).Warning("creating insecure application-server client")
	} else {
		log.WithField("server", hostname).Info("creating application-server client")
		cert, err := tls.X509KeyPair(tlsCert, tlsKey)
		if err != nil {
			return nil, errors.Wrap(err, "load x509 keypair error")
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.Wrap(err, "append ca cert to pool error")
		}

		asOpts = append(asOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
		})))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	asClient, err := grpc.DialContext(ctx, hostname, asOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "dial application-server api error")
	}

	return as.NewApplicationServerClient(asClient), nil
}
