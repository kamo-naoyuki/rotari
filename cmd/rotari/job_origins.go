package main

func queueOriginsByJobID(queue Queue) map[string]*JobOrigin {
	origins := make(map[string]*JobOrigin)
	for _, command := range queue.Commands {
		if command.Origin != nil {
			origins[command.ID] = command.Origin
		}
		for taskID, origin := range command.TaskOrigins {
			origins[taskID] = origin
		}
	}
	return origins
}
