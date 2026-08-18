package guardrail

type Direction string

const (
	DirectionInput       Direction = "input"
	DirectionOutput      Direction = "output"
	DirectionMCPRequest  Direction = "mcp_request"
	DirectionMCPResponse Direction = "mcp_response"
)

func (d Direction) String() string {
	return string(d)
}

