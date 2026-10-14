package httphandler_test

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kenyamaneko/overload-party-matchmaking/internal/adapter/httphandler/httphandlertest"
)

func TestHealth(t *testing.T) {
	t.Run("[ヘルスチェック]サーキットブレーカー状態の応答", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("X-Internal-Authヘッダを付けずにヘルスチェックを要求し、サーキットブレーカーが閉じているとき", func(t *testing.T) {
				t.Run("応答のステータスが200になる", func(t *testing.T) {
					router := NewServerWithCircuit(&FakeCircuit{Open: false})

					recorder := SendHealth(router)

					assert.Equal(t, 200, recorder.Code)
				})

				t.Run("応答本文のstatusがokになる", func(t *testing.T) {
					router := NewServerWithCircuit(&FakeCircuit{Open: false})

					recorder := SendHealth(router)

					status, _ := ReadHealth(t, recorder)
					assert.Equal(t, "ok", status)
				})

				t.Run("応答本文のcircuitがclosedになる", func(t *testing.T) {
					router := NewServerWithCircuit(&FakeCircuit{Open: false})

					recorder := SendHealth(router)

					_, circuit := ReadHealth(t, recorder)
					assert.Equal(t, "closed", circuit)
				})
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("サーキットブレーカーが開いているとき", func(t *testing.T) {
				t.Run("応答のステータスが、サーキットブレーカーが開いていることを意味する503になる", func(t *testing.T) {
					router := NewServerWithCircuit(&FakeCircuit{Open: true})

					recorder := SendHealth(router)

					assert.Equal(t, 503, recorder.Code)
				})

				t.Run("応答本文のstatusがdegradedになる", func(t *testing.T) {
					router := NewServerWithCircuit(&FakeCircuit{Open: true})

					recorder := SendHealth(router)

					status, _ := ReadHealth(t, recorder)
					assert.Equal(t, "degraded", status)
				})

				t.Run("応答本文のcircuitがopenになる", func(t *testing.T) {
					router := NewServerWithCircuit(&FakeCircuit{Open: true})

					recorder := SendHealth(router)

					_, circuit := ReadHealth(t, recorder)
					assert.Equal(t, "open", circuit)
				})
			})

			t.Run("1回目の要求ではサーキットブレーカーが開いていて、2回目の要求の前に閉じたとき", func(t *testing.T) {
				requestAfterClosing := func() *httptest.ResponseRecorder {
					circuit := &FakeCircuit{Open: true}
					router := NewServerWithCircuit(circuit)
					SendHealth(router)
					circuit.Open = false
					return SendHealth(router)
				}

				t.Run("2回目の応答のステータスが200になる", func(t *testing.T) {
					recorder := requestAfterClosing()

					assert.Equal(t, 200, recorder.Code)
				})

				t.Run("2回目の応答本文のstatusがokになる", func(t *testing.T) {
					recorder := requestAfterClosing()

					status, _ := ReadHealth(t, recorder)
					assert.Equal(t, "ok", status)
				})

				t.Run("2回目の応答本文のcircuitがclosedになる", func(t *testing.T) {
					recorder := requestAfterClosing()

					_, circuit := ReadHealth(t, recorder)
					assert.Equal(t, "closed", circuit)
				})
			})
		})
	})
}
