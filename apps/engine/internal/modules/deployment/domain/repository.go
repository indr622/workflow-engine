package domain

import "context"

// Repository adalah port penyimpanan process definition.
type Repository interface {
	// WithTx menjalankan fn dalam satu transaksi; rollback bila fn mengembalikan error.
	WithTx(ctx context.Context, fn func(ctx context.Context, tx TxRepository) error) error
	// GetByKey mengembalikan metadata dan konten resource asli.
	GetByKey(ctx context.Context, key string) (ProcessDefinition, []byte, error)
	// GetLatest mengembalikan versi terbaru beserta konten resource.
	GetLatest(ctx context.Context, tenantID, bpmnProcessID string) (ProcessDefinition, []byte, error)
	// GetByVersion mengembalikan versi tertentu beserta konten resource.
	GetByVersion(ctx context.Context, tenantID, bpmnProcessID string, version int32) (ProcessDefinition, []byte, error)
	// List mengembalikan halaman definition sesuai query.
	List(ctx context.Context, q ListQuery) ([]ProcessDefinition, error)
}

// TxRepository adalah operasi di dalam transaksi deploy.
type TxRepository interface {
	// LockProcessID menahan kunci per proses sampai transaksi selesai.
	LockProcessID(ctx context.Context, tenantID, bpmnProcessID string) error
	// LatestByProcessID mengembalikan versi terbaru; found=false bila belum pernah di-deploy.
	LatestByProcessID(ctx context.Context, tenantID, bpmnProcessID string) (pd ProcessDefinition, found bool, err error)
	// NextKeys menghasilkan n key baru.
	NextKeys(ctx context.Context, n int) ([]string, error)
	// SaveDeployment menyimpan deployment, resource, dan definition baru.
	SaveDeployment(ctx context.Context, d Deployment) error
}

// ListQuery adalah parameter keyset pagination daftar definition.
type ListQuery struct {
	TenantID       string
	BpmnProcessID  string
	LatestOnly     bool
	AfterProcessID string
	AfterVersion   int32
	Limit          int32
}
