package debezium

// CDCEvent represents raw CDC event from Debezium
type CDCEvent struct {
	Before      map[string]interface{} `json:"before"`
	After       map[string]interface{} `json:"after"`
	Source      CDCSource              `json:"source"`
	Operation   string                 `json:"op"` // c=create, u=update, d=delete, r=read
	TsMs        int64                  `json:"ts_ms"`
	Transaction interface{}            `json:"transaction"`
}

type CDCField struct {
	Field    string `json:"field"`
	Type     string `json:"type"`
	Optional bool   `json:"optional"`
}

type CDCSource struct {
	Version   string `json:"version"`
	Connector string `json:"connector"`
	Name      string `json:"name"`
	TsMs      int64  `json:"ts_ms"`
	Snapshot  string `json:"snapshot"`
	DB        string `json:"db"`
	Sequence  string `json:"sequence"`
	Schema    string `json:"schema"`
	Table     string `json:"table"`
	TxID      int64  `json:"txId"`
	LSN       int64  `json:"lsn"`
	XMIN      int64  `json:"xmin"`
}
