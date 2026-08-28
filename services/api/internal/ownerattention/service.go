package ownerattention

import (
	"context"
	"errors"
	"time"
)

const maximumItems = 50

var ErrUnauthenticated = errors.New("authenticated owner required")
var ErrUnavailable = errors.New("owner attention is unavailable")

type Store interface {
	Items(context.Context, string, int) ([]Item, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (service *Service) Overview(ctx context.Context, userID string) (Overview, error) {
	if userID == "" {
		return Overview{}, ErrUnauthenticated
	}
	if service.store == nil {
		return Overview{}, ErrUnavailable
	}
	items, err := service.store.Items(ctx, userID, maximumItems)
	if err != nil {
		return Overview{}, err
	}
	if items == nil {
		items = []Item{}
	}
	if len(items) > maximumItems {
		items = items[:maximumItems]
	}
	overview := Overview{
		GeneratedAt: service.now().UTC(),
		Status:      StatusClear,
		Items:       items,
		Total:       len(items),
	}
	for _, item := range items {
		switch item.Severity {
		case SeverityAttention:
			overview.AttentionCount++
			if overview.Status == StatusClear {
				overview.Status = StatusAttention
			}
		case SeverityStopped:
			overview.StoppedCount++
			overview.Status = StatusStopped
		default:
			return Overview{}, ErrUnavailable
		}
	}
	return overview, nil
}
