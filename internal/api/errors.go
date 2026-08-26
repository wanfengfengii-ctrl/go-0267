package api

import "sort"

// Reason is one sorted rejection reason. The stable sort order is house, batch
// number, tray seal, position, detection well, then error code.
type Reason struct {
	HouseID  string `json:"house_id,omitempty"`
	BatchNo  string `json:"batch_no,omitempty"`
	TraySeal string `json:"tray_seal,omitempty"`
	Position int    `json:"position,omitempty"`
	Well     string `json:"well,omitempty"`
	Code     string `json:"code"`
}

// ErrorResponse is the uniform error envelope for every endpoint.
type ErrorResponse struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	Reasons     []Reason `json:"reasons,omitempty"`
	OperationID string   `json:"operation_id,omitempty"`
}

// SortReasons orders reasons deterministically so that map iteration, request
// order and concurrent scheduling never change the response.
func SortReasons(rs []Reason) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.HouseID != b.HouseID {
			return a.HouseID < b.HouseID
		}
		if a.BatchNo != b.BatchNo {
			return a.BatchNo < b.BatchNo
		}
		if a.TraySeal != b.TraySeal {
			return a.TraySeal < b.TraySeal
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		if a.Well != b.Well {
			return a.Well < b.Well
		}
		return a.Code < b.Code
	})
}
