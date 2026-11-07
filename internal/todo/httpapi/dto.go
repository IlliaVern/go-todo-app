package httpapi

import "github.com/IlliaVern/go-todo-app/internal/todo/domain"

// createRequest is the body for creating a list or an item.
type createRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type updateListRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

type updateItemRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Done        *bool   `json:"done"`
}

type listResponse struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type listsResponse struct {
	Data []listResponse `json:"data"`
}

type itemResponse struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Done        bool   `json:"done"`
}

func toListResponse(l domain.List) listResponse {
	return listResponse{ID: l.ID(), Title: l.Title().String(), Description: l.Description().String()}
}

func toListResponses(lists []domain.List) []listResponse {
	out := make([]listResponse, 0, len(lists))
	for _, l := range lists {
		out = append(out, toListResponse(l))
	}
	return out
}

func toItemResponse(i domain.Item) itemResponse {
	return itemResponse{ID: i.ID(), Title: i.Title().String(), Description: i.Description().String(), Done: i.Done()}
}

func toItemResponses(items []domain.Item) []itemResponse {
	out := make([]itemResponse, 0, len(items))
	for _, i := range items {
		out = append(out, toItemResponse(i))
	}
	return out
}
