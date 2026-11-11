package provision

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/qzytwway/mplatform/pkg/cluster"
	"github.com/qzytwway/mplatform/pkg/model"
	"gorm.io/gorm"
)

var errNotFound = errors.New("installation not found")

// conflictError is returned as HTTP 409.
type conflictError struct{ message string }

func (e *conflictError) Error() string { return e.message }

func conflictf(format string, args ...any) error {
	return &conflictError{message: fmt.Sprintf(format, args...)}
}

func isConflict(err error) bool {
	var conflict *conflictError
	return errors.As(err, &conflict)
}

// createProvisionRecord stores a pending provision with one node row per
// spec node holding its effective SSH credentials.
func createProvisionRecord(spec Spec, kubeVersion, createdBy string) (*model.ClusterProvision, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	specJSON, err := json.Marshal(spec.Sanitized())
	if err != nil {
		return nil, err
	}
	p := &model.ClusterProvision{
		Name:              spec.Name,
		Description:       spec.Description,
		KubernetesMinor:   spec.KubernetesMinor,
		KubernetesVersion: kubeVersion,
		ControlPlaneMode:  spec.ControlPlane.Mode,
		Spec:              specJSON,
		Status:            model.ProvisionStatusPending,
		Steps:             encodeSteps(initialSteps(spec)),
		EncryptionKey:     model.SecretString(base64.StdEncoding.EncodeToString(key)),
		CreatedBy:         createdBy,
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := nameAvailable(tx, spec.Name); err != nil {
			return err
		}
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		nodes := make([]model.ProvisionNode, len(spec.Nodes))
		for i, n := range spec.Nodes {
			s := spec.EffectiveSSH(n)
			nodes[i] = model.ProvisionNode{
				ProvisionID:   p.ID,
				IP:            n.IP,
				Hostname:      n.Hostname,
				Role:          n.Role,
				SSHUser:       s.User,
				SSHPort:       s.Port,
				SSHAuthType:   s.AuthType,
				SSHPrivateKey: model.SecretString(s.PrivateKey),
				SSHPassword:   model.SecretString(s.Password),
				Status:        statusPending,
			}
		}
		return tx.Create(&nodes).Error
	})
	if err != nil {
		// A concurrent create with the same name fails on the unique index.
		if nameErr := nameAvailable(model.DB, spec.Name); isConflict(nameErr) && !isConflict(err) {
			return nil, nameErr
		}
		return nil, err
	}
	return p, nil
}

func nameAvailable(tx *gorm.DB, name string) error {
	var count int64
	if err := tx.Model(&model.Cluster{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return conflictf("a cluster named %s already exists", name)
	}
	if err := tx.Model(&model.ClusterProvision{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return conflictf("an installation named %s already exists", name)
	}
	return nil
}

func findProvision(tx *gorm.DB, id uint) (*model.ClusterProvision, error) {
	var p model.ClusterProvision
	if err := tx.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errNotFound
		}
		return nil, err
	}
	return &p, nil
}

func leaseHeld(p *model.ClusterProvision, now time.Time) bool {
	return p.LockUntil != nil && p.LockUntil.After(now)
}

// retryProvision resumes a failed or cancelled provision from the step that
// stopped; finished steps are kept.
func retryProvision(id uint) error {
	now := time.Now()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		p, err := findProvision(tx, id)
		if err != nil {
			return err
		}
		if p.Status != model.ProvisionStatusFailed && p.Status != model.ProvisionStatusCancelled {
			return conflictf("only failed or cancelled installations can be retried")
		}
		if leaseHeld(p, now) {
			return conflictf("the previous run is still stopping; try again in a moment")
		}
		steps := decodeSteps(p)
		for i := range steps {
			if steps[i].Status == statusFailed || steps[i].Status == statusRunning {
				steps[i] = stepState{Name: steps[i].Name, Status: statusPending}
			}
		}
		result := tx.Model(&model.ClusterProvision{}).Where("id = ? AND status = ?", id, p.Status).Updates(map[string]any{
			"status":       model.ProvisionStatusPending,
			"error":        "",
			"current_step": "",
			"steps":        encodeSteps(steps),
			"finished_at":  nil,
			"locked_by":    "",
			"lock_until":   nil,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return conflictf("the installation changed; reload and try again")
		}
		return tx.Model(&model.ProvisionNode{}).
			Where("provision_id = ? AND status IN ?", id, []string{statusFailed, statusRunning}).
			Updates(map[string]any{"status": statusPending, "current_action": "", "error": ""}).Error
	})
}

// cancelProvision marks a pending or running provision cancelled; the
// runner stops at once on this instance or at its next heartbeat elsewhere.
func cancelProvision(id uint) error {
	result := model.DB.Model(&model.ClusterProvision{}).
		Where("id = ? AND status IN ?", id, []string{model.ProvisionStatusPending, model.ProvisionStatusRunning}).
		Updates(map[string]any{"status": model.ProvisionStatusCancelled, "error": cancelledMessage, "finished_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	if _, err := findProvision(model.DB, id); err != nil {
		return err
	}
	return conflictf("only pending or running installations can be cancelled")
}

// deleteProvision either schedules a node reset (reset=true) or removes the
// records right away.
func deleteProvision(id uint, reset bool) error {
	now := time.Now()
	p, err := findProvision(model.DB, id)
	if err != nil {
		return err
	}
	if p.Status == model.ProvisionStatusRunning || p.Status == model.ProvisionStatusDeleting || leaseHeld(p, now) {
		return conflictf("the installation is %s; cancel it or wait before deleting", p.Status)
	}
	if p.ClusterID != nil {
		if c, err := model.GetClusterByID(*p.ClusterID); err == nil && c.IsDefault {
			return errDefaultCluster
		}
	}
	if !reset {
		return deleteProvisionRecords(id, p.ClusterID, "")
	}
	result := model.DB.Model(&model.ClusterProvision{}).
		Where("id = ? AND status = ? AND (lock_until IS NULL OR lock_until < ?)", id, p.Status, now).
		Updates(map[string]any{"status": model.ProvisionStatusDeleting, "error": "", "current_step": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return conflictf("the installation changed; reload and try again")
	}
	return nil
}

var errDefaultCluster = errors.New("cannot delete default cluster")

// deleteProvisionRecords removes the platform cluster, the provision, its
// nodes and logs. lockedBy guards deletes performed by a runner.
func deleteProvisionRecords(id uint, clusterID *uint, lockedBy string) error {
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ?", id)
		if lockedBy != "" {
			query = query.Where("locked_by = ?", lockedBy)
		} else {
			query = query.Where("status NOT IN ?", []string{model.ProvisionStatusRunning, model.ProvisionStatusDeleting})
		}
		result := query.Delete(&model.ClusterProvision{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return conflictf("the installation changed; reload and try again")
		}
		if err := tx.Where("provision_id = ?", id).Delete(&model.ProvisionNode{}).Error; err != nil {
			return err
		}
		if err := tx.Where("provision_id = ?", id).Delete(&model.ProvisionLog{}).Error; err != nil {
			return err
		}
		if clusterID != nil {
			return tx.Delete(&model.Cluster{}, *clusterID).Error
		}
		return nil
	})
	if err == nil && clusterID != nil {
		cluster.TriggerClusterSync()
	}
	return err
}
