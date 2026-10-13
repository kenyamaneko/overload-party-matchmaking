// Package pubsubtest は Pub/Sub エミュレータの起動と match_made の本文の組み立てを提供するテスト用の補助部品。
package pubsubtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	gpubsub "cloud.google.com/go/pubsub/v2"
	pubsubpb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ErrTimeout は WaitForN が待ち時間の上限に達したときに返る。
var ErrTimeout = errors.New("pubsubtest: timeout waiting for message")

const (
	emulatorImage          = "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators"
	emulatorPortNumber     = "8085"
	emulatorPort           = emulatorPortNumber + "/tcp"
	emulatorLogReady       = "Server started, listening on " + emulatorPortNumber
	emulatorStartupTimeout = 90 * time.Second

	// receivedMessageBufferSize は 1 件のテストが受け取るメッセージ数を十分に上回る値にする。
	receivedMessageBufferSize = 64

	idSuffixLength = 8
)

// Emulator は起動済みの Pub/Sub エミュレータのコンテナを指す。
type Emulator struct {
	container testcontainers.Container
	projectID string
	client    *gpubsub.Client
}

// StartEmulator は Pub/Sub エミュレータのコンテナを起動し、環境変数 PUBSUB_EMULATOR_HOST に接続先を設定する。
func StartEmulator(ctx context.Context, projectID string) (*Emulator, error) {
	req := testcontainers.ContainerRequest{
		Image:        emulatorImage,
		ExposedPorts: []string{emulatorPort},
		Cmd: []string{
			"gcloud", "beta", "emulators", "pubsub", "start",
			"--host-port=0.0.0.0:" + emulatorPortNumber,
			"--project=" + projectID,
		},
		WaitingFor: wait.ForLog(emulatorLogReady).WithStartupTimeout(emulatorStartupTimeout),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start pubsub emulator container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("container host: %w", err), container.Terminate(ctx))
	}
	mapped, err := container.MappedPort(ctx, emulatorPortNumber)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("mapped port: %w", err), container.Terminate(ctx))
	}
	endpoint := fmt.Sprintf("%s:%s", host, mapped.Port())

	// 本番コードの gpubsub.NewClient(projectID) を変更せずにエミュレータへ接続させるため、環境変数で接続先を渡す。
	if err := os.Setenv("PUBSUB_EMULATOR_HOST", endpoint); err != nil {
		return nil, errors.Join(fmt.Errorf("setenv PUBSUB_EMULATOR_HOST: %w", err), container.Terminate(ctx))
	}

	client, err := newEmulatorClient(ctx, projectID, endpoint)
	if err != nil {
		return nil, errors.Join(err, container.Terminate(ctx))
	}

	return &Emulator{
		container: container,
		projectID: projectID,
		client:    client,
	}, nil
}

func newEmulatorClient(ctx context.Context, projectID, endpoint string) (*gpubsub.Client, error) {
	opts := []option.ClientOption{
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	client, err := gpubsub.NewClient(ctx, projectID, opts...)
	if err != nil {
		return nil, fmt.Errorf("new pubsub client: %w", err)
	}
	return client, nil
}

// Close はクライアントとコンテナを停止し、環境変数 PUBSUB_EMULATOR_HOST を解除する。
func (e *Emulator) Close(ctx context.Context) error {
	var errs []error
	if err := e.client.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close client: %w", err))
	}
	if err := e.container.Terminate(ctx); err != nil {
		errs = append(errs, fmt.Errorf("terminate container: %w", err))
	}
	if err := os.Unsetenv("PUBSUB_EMULATOR_HOST"); err != nil {
		errs = append(errs, fmt.Errorf("unsetenv PUBSUB_EMULATOR_HOST: %w", err))
	}
	return errors.Join(errs...)
}

// GenerateTopicName は接頭辞から一意なトピック名を生成する。トピックは作成しない。
func GenerateTopicName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.NewString()[:idSuffixLength])
}

// CreateTopic は一意な名前のトピックを作成し、その名前を返す。
func (e *Emulator) CreateTopic(t *testing.T, prefix string) string {
	t.Helper()
	topicName := GenerateTopicName(prefix)
	e.CreateTopicWithName(t, topicName)
	return topicName
}

// CreateTopicWithName は指定した名前のトピックを作成する。
func (e *Emulator) CreateTopicWithName(t *testing.T, topicName string) {
	t.Helper()
	ctx := context.Background()
	_, err := e.client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{
		Name: fmt.Sprintf("projects/%s/topics/%s", e.projectID, topicName),
	})
	if err != nil {
		t.Fatalf("create topic %s: %v", topicName, err)
	}
}

// Subscribe は指定したトピックに一意な名前の購読を作成し、受信を開始する。受信したメッセージは確認応答済みにする。
func (e *Emulator) Subscribe(t *testing.T, topicName string) *Subscription {
	t.Helper()

	subscriptionName := fmt.Sprintf("%s-sub-%s", topicName, uuid.NewString()[:idSuffixLength])
	ctx := context.Background()
	_, err := e.client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:  fmt.Sprintf("projects/%s/subscriptions/%s", e.projectID, subscriptionName),
		Topic: fmt.Sprintf("projects/%s/topics/%s", e.projectID, topicName),
	})
	if err != nil {
		t.Fatalf("create subscription %s: %v", subscriptionName, err)
	}

	// テストが読み出すまでコールバックがブロックしないよう、バッファ付きのチャンネルにする。
	ch := make(chan *Message, receivedMessageBufferSize)
	receiveCtx, cancel := context.WithCancel(context.Background())
	s := &Subscription{
		ch:     ch,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	subscriber := e.client.Subscriber(subscriptionName)
	go func() {
		defer close(s.done)
		s.receiveErr = subscriber.Receive(receiveCtx, func(_ context.Context, m *gpubsub.Message) {
			m.Ack()
			select {
			case ch <- &Message{Data: m.Data}:
			case <-receiveCtx.Done():
			}
		})
	}()

	t.Cleanup(func() {
		s.stop()
		if err := e.client.SubscriptionAdminClient.DeleteSubscription(context.Background(), &pubsubpb.DeleteSubscriptionRequest{
			Subscription: fmt.Sprintf("projects/%s/subscriptions/%s", e.projectID, subscriptionName),
		}); err != nil {
			t.Errorf("delete subscription %s: %v", subscriptionName, err)
		}
	})
	return s
}

// Message は購読で受信したメッセージ。
type Message struct {
	Data []byte
}

// Subscription は購読の受信状態を指す。
type Subscription struct {
	ch     chan *Message
	cancel context.CancelFunc
	done   chan struct{}

	// done を閉じる前に書き込むため、done を受信した側は競合なく読める。
	receiveErr error
	stopOnce   sync.Once
}

func (s *Subscription) stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		<-s.done
	})
}

// WaitForN は待ち時間の上限以内に n 件届くのを待つ。不足したときは届いた分と ErrTimeout を返す。受信が失敗して止まったときは、届いた分と受信の失敗を返す。
func (s *Subscription) WaitForN(n int, timeout time.Duration) ([]*Message, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	msgs := make([]*Message, 0, n)
	for len(msgs) < n {
		select {
		case msg := <-s.ch:
			msgs = append(msgs, msg)
		case <-s.done:
			return msgs, fmt.Errorf("pubsubtest: receive stopped: %w", s.receiveErr)
		case <-timer.C:
			return msgs, ErrTimeout
		}
	}
	return msgs, nil
}
