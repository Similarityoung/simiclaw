package payload

type messageHandler struct{}

func (messageHandler) PayloadType() string {
	return "message"
}

func (messageHandler) Plan() Plan {
	return interactivePlan()
}
