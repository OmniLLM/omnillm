package routes

import (
	"fmt"
	"omnillm/internal/database"
	providertypes "omnillm/internal/providers/types"
)

type providerInstanceReservation struct {
	instanceID string
	created    bool
}

func reserveProviderInstance(provider providertypes.Provider) (*providerInstanceReservation, error) {
	instanceID := provider.GetInstanceID()
	created, err := database.NewProviderInstanceStore().Create(&database.ProviderInstanceRecord{
		InstanceID: instanceID,
		ProviderID: provider.GetID(),
		Name:       provider.GetName(),
	})
	if err != nil {
		return nil, fmt.Errorf("reserve provider instance: %w", err)
	}

	return &providerInstanceReservation{instanceID: instanceID, created: created}, nil
}

func createProvider(provider providertypes.Provider, create func() error) error {
	reservation, err := reserveProviderInstance(provider)
	if err != nil {
		return err
	}
	defer reservation.rollback()

	if err := create(); err != nil {
		return err
	}
	reservation.commit()
	return nil
}

func (r *providerInstanceReservation) rollback() {
	if r == nil || !r.created {
		return
	}
	_ = database.NewProviderInstanceStore().Delete(r.instanceID)
	r.created = false
}

func (r *providerInstanceReservation) commit() {
	if r != nil {
		r.created = false
	}
}
