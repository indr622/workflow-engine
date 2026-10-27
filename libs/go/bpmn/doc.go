// Package bpmn adalah kernel murni untuk membaca, memodelkan, dan memvalidasi BPMN 2.0
// yang kompatibel dengan Camunda Modeler (execution platform Camunda 8, ekstensi zeebe:).
//
// Paket ini hanya memakai standard library. Alurnya:
//
//	defs, err := bpmn.Parse(r)              // error hanya untuk kegagalan fatal
//	issues := bpmn.Validate(defs, bpmn.ProfileExecutable)
//	if issues.HasErrors() { ... }
//
// Model yang dihasilkan Parse bersifat immutable: field tidak diekspor dan hanya bisa dibaca
// lewat getter, sehingga aman dibaca bersamaan oleh banyak goroutine tanpa lock. Slice yang
// dikembalikan getter berbagi backing array dengan model dan tidak boleh diubah pemanggil.
//
// Diagram interchange (BPMNDI) tidak dimodelkan; simpan XML asli bila diagram dibutuhkan.
package bpmn
