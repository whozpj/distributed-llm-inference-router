package router

import "hash/fnv"

func HashPrefix(tokens []int32, n int) uint64 {
	if n > len(tokens) {
		n = len(tokens)
	}
	h := fnv.New64a()
	for _, t := range tokens[:n] {
		h.Write([]byte{byte(t), byte(t >> 8), byte(t >> 16), byte(t >> 24)})
	}
	return h.Sum64()
}
