package testgrpc

type StreamServer struct {
	UnsafeStreamServiceServer

	ServerHandler func(stream StreamService_StreamServer) error
}

func (s *StreamServer) Stream(stream StreamService_StreamServer) error {
	if s.ServerHandler != nil {
		return s.ServerHandler(stream)
	}
	return nil
}
