package routes

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"omnillm/internal/database"
)

func TestRandomProviderReservationRetriesCollisions(t *testing.T) {
	const existingID = "9ef98730-3232-42f3-987d-3114e8adad10"
	const availableID = "9ef98730-3232-42f3-987d-3114e8adad11"
	store := database.NewProviderInstanceStore()
	if _, err := store.Create(&database.ProviderInstanceRecord{
		InstanceID: existingID, ProviderID: "alibaba", Name: "Existing",
	}); err != nil {
		t.Fatalf("seed existing provider: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Delete(existingID)
		_ = store.Delete(availableID)
	})
	ids := []uuid.UUID{uuid.MustParse(existingID), uuid.MustParse(availableID)}
	index := 0
	reservation, err := reserveRandomProviderWith("alibaba", func() (uuid.UUID, error) {
		id := ids[index]
		index++
		return id, nil
	})
	if err != nil {
		t.Fatalf("reserve provider: %v", err)
	}
	if reservation.instanceID != availableID || index != 2 {
		t.Fatalf("reservation ID %q after %d attempts", reservation.instanceID, index)
	}
	if err := reservation.rollback(); err != nil {
		t.Fatalf("rollback reservation: %v", err)
	}
	existing, err := store.Get(existingID)
	if err != nil || existing == nil || existing.Name != "Existing" {
		t.Fatalf("existing provider after collision: %#v, %v", existing, err)
	}
	reserved, err := store.Get(availableID)
	if err != nil || reserved != nil {
		t.Fatalf("rolled-back reservation: %#v, %v", reserved, err)
	}
}

func TestRandomProviderReservationFailsWhenRandomnessFails(t *testing.T) {
	wantErr := errors.New("randomness unavailable")
	_, err := reserveRandomProviderWith("alibaba", func() (uuid.UUID, error) {
		return uuid.Nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("reserve provider error = %v, want randomness error", err)
	}
}

func TestRandomProviderReservationAvoidsExistingAlias(t *testing.T) {
	const aliasID = "9ef98730-3232-42f3-987d-3114e8adad20"
	const availableID = "9ef98730-3232-42f3-987d-3114e8adad21"
	const ownerID = "uuid-alias-owner"
	store := database.NewProviderInstanceStore()
	if _, err := store.Create(&database.ProviderInstanceRecord{
		InstanceID: ownerID, ProviderID: "alibaba", Name: "Existing", Subtitle: strings.ToUpper(aliasID),
	}); err != nil {
		t.Fatalf("seed alias owner: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Delete(ownerID)
		_ = store.Delete(availableID)
	})
	ids := []uuid.UUID{uuid.MustParse(aliasID), uuid.MustParse(availableID)}
	index := 0
	reservation, err := reserveRandomProviderWith("alibaba", func() (uuid.UUID, error) {
		id := ids[index]
		index++
		return id, nil
	})
	if err != nil {
		t.Fatalf("reserve provider: %v", err)
	}
	defer cleanupProviderReservation(reservation)
	if reservation.instanceID != availableID || index != 2 {
		t.Fatalf("reservation ID %q after %d attempts", reservation.instanceID, index)
	}
}

func TestRandomProviderReservationsAreUniqueConcurrently(t *testing.T) {
	const count = 32
	type result struct {
		reservation *providerInstanceReservation
		err         error
	}
	results := make(chan result, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			reservation, err := reserveRandomProvider("alibaba")
			results <- result{reservation: reservation, err: err}
		})
	}
	wg.Wait()
	close(results)
	seen := make(map[string]bool, count)
	reservations := make([]*providerInstanceReservation, 0, count)
	t.Cleanup(func() {
		for _, reservation := range reservations {
			if err := reservation.rollback(); err != nil {
				t.Errorf("rollback reservation: %v", err)
			}
		}
	})
	for item := range results {
		if item.err != nil {
			t.Errorf("reserve provider: %v", item.err)
			continue
		}
		reservations = append(reservations, item.reservation)
		id := item.reservation.instanceID
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.Version() != 4 || parsed.String() != id || seen[id] {
			t.Errorf("invalid or duplicate provider id %q", id)
		}
		seen[id] = true
	}
	if len(seen) != count {
		t.Fatalf("reserved %d unique ids, want %d", len(seen), count)
	}
}
