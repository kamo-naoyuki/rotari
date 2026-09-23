package server

import (
	"encoding/json"
	"net"
	"time"
)

func SendRequest(baseDir string, request Request) (Response, error) {
	conn, err := net.DialTimeout("unix", SocketPath(baseDir), time.Second)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return Response{}, err
	}
	return response, nil
}
