package routes

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
	providertypes "omnillm/internal/providers/types"
	"omnillm/internal/registry"
)

type providerInstanceReservation struct {
	instanceID string
	created    bool
}

func reserveRandomProvider(providerType string) (*providerInstanceReservation, error) {
	return reserveRandomProviderWith(providerType, uuid.NewRandom)
}

func reserveRandomProviderWith(providerType string, generate func() (uuid.UUID, error)) (*providerInstanceReservation, error) {
	const maxAttempts = 16
	store := database.NewProviderInstanceStore()
	reg := registry.GetProviderRegistry()
	for range maxAttempts {
		id, err := generate()
		if err != nil {
			return nil, fmt.Errorf("generate provider id: %w", err)
		}
		instanceID := id.String()
		providerMetadataMu.Lock()
		if reg.IsRegistered(instanceID) {
			providerMetadataMu.Unlock()
			continue
		}
		records, err := store.GetAll()
		if err != nil {
			providerMetadataMu.Unlock()
			return nil, fmt.Errorf("check provider identities: %w", err)
		}
		conflict := false
		for _, record := range records {
			if record.InstanceID == instanceID || (record.Subtitle != "" && strings.EqualFold(record.Subtitle, instanceID)) || (record.Name != "" && strings.EqualFold(record.Name, instanceID)) {
				conflict = true
				break
			}
		}
		if conflict {
			providerMetadataMu.Unlock()
			continue
		}
		created, err := store.Create(&database.ProviderInstanceRecord{
			InstanceID: instanceID,
			ProviderID: providerType,
		})
		providerMetadataMu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("reserve provider id: %w", err)
		}
		if created {
			return &providerInstanceReservation{instanceID: instanceID, created: true}, nil
		}
	}
	return nil, fmt.Errorf("unable to reserve a unique provider id")
}

func createReservedProvider(
	reservation *providerInstanceReservation,
	provider providertypes.Provider,
	authenticate func() error,
) (resultErr error) {
	if reservation == nil || !reservation.created || reservation.instanceID != provider.GetInstanceID() {
		return fmt.Errorf("invalid provider reservation")
	}
	defer func() {
		if cleanupErr := reservation.rollback(); cleanupErr != nil {
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()
	if err := authenticate(); err != nil {
		return err
	}
	providerMetadataMu.Lock()
	defer providerMetadataMu.Unlock()
	name := provider.GetName()
	if err := database.NewProviderInstanceStore().UpdateMetadata(reservation.instanceID, &name, nil); err != nil {
		return fmt.Errorf("save provider metadata: %w", err)
	}
	if err := registry.GetProviderRegistry().RegisterNew(provider, true); err != nil {
		return fmt.Errorf("register provider: %w", err)
	}
	reservation.commit()
	return nil
}

func (r *providerInstanceReservation) rollback() error {
	if r == nil || !r.created {
		return nil
	}
	if err := database.NewProviderInstanceStore().Delete(r.instanceID); err != nil {
		return fmt.Errorf("remove reserved provider: %w", err)
	}
	r.created = false
	return nil
}

func cleanupProviderReservation(reservation *providerInstanceReservation) {
	if err := reservation.rollback(); err != nil {
		log.Error().Err(err).Msg("Failed to clean up reserved provider")
	}
}

func (r *providerInstanceReservation) commit() {
	if r != nil {
		r.created = false
	}
}
