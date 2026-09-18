package workspace

import "encoding/json"

// Encode serializes a State to indented JSON.
func Encode(s *State) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// Decode parses a State from JSON produced by Encode.
func Decode(data []byte) (*State, error) {
	s := new(State)
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}
