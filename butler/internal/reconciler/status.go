package reconciler

import "github.com/iamkhattar/homelab/butler/internal/platform"

func convergeStatus(current *platform.ResourceStatus, desired platform.ResourceStatus, update func() error) error {
	next, changed := platform.ConvergeStatus(*current, desired)
	if !changed {
		return nil
	}
	*current = next
	return update()
}
