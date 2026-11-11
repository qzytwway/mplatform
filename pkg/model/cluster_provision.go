package model

import "time"

const (
	ProvisionStatusPending   = "pending"
	ProvisionStatusRunning   = "running"
	ProvisionStatusSucceeded = "succeeded"
	ProvisionStatusFailed    = "failed"
	ProvisionStatusCancelled = "cancelled"
	ProvisionStatusDeleting  = "deleting"
)

// ClusterProvision is one installation of a kubeadm cluster on user hosts.
// Spec holds the requested specification with every SSH secret stripped;
// credentials live on ProvisionNode.
type ClusterProvision struct {
	Model
	Name              string       `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"`
	Description       string       `json:"description" gorm:"type:text"`
	KubernetesMinor   string       `json:"kubernetesMinor" gorm:"type:varchar(10)"`
	KubernetesVersion string       `json:"kubernetesVersion" gorm:"type:varchar(32)"`
	ControlPlaneMode  string       `json:"controlPlaneMode" gorm:"type:varchar(20)"`
	Spec              JSONField    `json:"-" gorm:"type:text"`
	Status            string       `json:"status" gorm:"type:varchar(20);not null;index"`
	CurrentStep       string       `json:"currentStep" gorm:"type:varchar(40)"`
	Steps             JSONField    `json:"-" gorm:"type:text"`
	Error             string       `json:"error" gorm:"type:text"`
	EncryptionKey     SecretString `json:"-" gorm:"type:text"`
	ClusterID         *uint        `json:"clusterId" gorm:"index"`
	CreatedBy         string       `json:"createdBy" gorm:"type:varchar(255)"`
	StartedAt         *time.Time   `json:"startedAt"`
	FinishedAt        *time.Time   `json:"finishedAt"`
	LockedBy          string       `json:"-" gorm:"type:varchar(255)"`
	LockUntil         *time.Time   `json:"-" gorm:"index"`
}

// ProvisionNode is one host of a ClusterProvision with its effective SSH
// settings. HostKey is recorded on first connect and verified afterwards.
type ProvisionNode struct {
	Model
	ProvisionID   uint         `json:"provisionId" gorm:"not null;index"`
	IP            string       `json:"ip" gorm:"type:varchar(64);not null"`
	Hostname      string       `json:"hostname" gorm:"type:varchar(255)"`
	Role          string       `json:"role" gorm:"type:varchar(20);not null"`
	SSHUser       string       `json:"-" gorm:"type:varchar(255)"`
	SSHPort       int          `json:"-"`
	SSHAuthType   string       `json:"-" gorm:"type:varchar(20)"`
	SSHPrivateKey SecretString `json:"-" gorm:"type:text"`
	SSHPassword   SecretString `json:"-" gorm:"type:text"`
	HostKey       string       `json:"-" gorm:"type:text"`
	Facts         JSONField    `json:"-" gorm:"type:text"`
	Precheck      JSONField    `json:"-" gorm:"type:text"`
	Joined        bool         `json:"-" gorm:"type:boolean;default:false"`
	Status        string       `json:"status" gorm:"type:varchar(20)"`
	CurrentAction string       `json:"currentAction" gorm:"type:varchar(255)"`
	Error         string       `json:"error" gorm:"type:text"`
}

// ProvisionLog is one output line of a provision, read incrementally by ID.
type ProvisionLog struct {
	ID          uint      `json:"id" gorm:"primarykey"`
	ProvisionID uint      `json:"provisionId" gorm:"not null;index"`
	NodeID      *uint     `json:"nodeId" gorm:"index"`
	Node        string    `json:"node" gorm:"type:varchar(255)"`
	Step        string    `json:"step" gorm:"type:varchar(40)"`
	Text        string    `json:"text" gorm:"type:text"`
	CreatedAt   time.Time `json:"time"`
}
