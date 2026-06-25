package router

type Request struct {
	ID           string
	PromptTokens []int32
	MaxNewTokens int32
}

type Policy interface {
	Pick(req *Request, replicas []*ReplicaClient) *ReplicaClient
}
