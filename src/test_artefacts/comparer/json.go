package comparer

import (
	"encoding/json"
	"reflect"

	"github.com/google/go-cmp/cmp"
)

// JSONRawMessage compara json.RawMessage ignorando a ordem das chaves
func JSONRawMessage() cmp.Option {
	return cmp.Comparer(func(x, y json.RawMessage) bool {
		// Se ambos são nil ou vazios
		if len(x) == 0 && len(y) == 0 {
			return true
		}

		// Se um é vazio e o outro não
		if len(x) == 0 || len(y) == 0 {
			return false
		}

		// Parse ambos para interface{} para comparação semântica
		var xObj, yObj interface{}

		if err := json.Unmarshal(x, &xObj); err != nil {
			return false
		}

		if err := json.Unmarshal(y, &yObj); err != nil {
			return false
		}

		// Usa reflect.DeepEqual para comparação semântica
		return reflect.DeepEqual(xObj, yObj)
	})
}