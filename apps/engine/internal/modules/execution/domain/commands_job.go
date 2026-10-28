package domain

import "fmt"

// Command protokol job worker dan operator (Fase 4): gagal, lempar error, ubah retries,
// selesaikan incident, dan set variabel.

// defaultNoRetriesMessage dipakai Zeebe bila worker tidak memberi pesan error.
const defaultNoRetriesMessage = "No more retries left."

func (r *run) failJob(c FailJob) error {
	job, el, err := r.activatableJob(c.JobKey)
	if err != nil {
		return err
	}
	v := r.jobValue(job)
	v.Retries = c.Retries
	v.ErrorMessage = c.ErrorMessage
	if c.Retries > 0 && c.RetryBackoff > 0 {
		v.RetryBackoffMs = c.RetryBackoff.Milliseconds()
	}
	if err := r.emit(job.Key, ValueJob, IntentFailed, v); err != nil {
		return err
	}
	// Belum ada scope lokal (Fase 7), jadi variabel fail ditulis ke scope process instance.
	if err := r.mergeVariables(c.Variables); err != nil {
		return err
	}
	if c.Retries > 0 {
		return nil
	}
	msg := c.ErrorMessage
	if msg == "" {
		msg = defaultNoRetriesMessage
	}
	return r.raiseJobIncident(el, IncidentJobNoRetries, msg, job.Key)
}

func (r *run) throwError(c ThrowError) error {
	job, el, err := r.activatableJob(c.JobKey)
	if err != nil {
		return err
	}
	v := r.jobValue(job)
	v.ErrorCode = c.ErrorCode
	v.ErrorMessage = c.ErrorMessage
	if err := r.emit(job.Key, ValueJob, IntentErrorThrown, v); err != nil {
		return err
	}
	if err := r.mergeVariables(c.Variables); err != nil {
		return err
	}
	if r.findErrorCatch(el, c.ErrorCode) {
		return nil
	}
	return r.raiseJobIncident(el, IncidentUnhandledErrorEvent, fmt.Sprintf(
		"Expected to throw an error event with the code '%s' with message '%s', but it was not caught. No error events are available in the scope.",
		c.ErrorCode, c.ErrorMessage), job.Key)
}

// findErrorCatch adalah titik sisip Fase 5 (boundary error event dan event sub-process). Fase 4
// belum mengeksekusi catch event, jadi error tidak pernah tertangkap.
func (r *run) findErrorCatch(_ *ElementInstance, _ string) bool { return false }

func (r *run) updateJobRetries(c UpdateJobRetries) error {
	job, ok := r.st.Jobs[c.JobKey]
	if !ok {
		return ErrJobNotOpen
	}
	v := r.jobValue(job)
	v.Retries = c.Retries
	return r.emit(job.Key, ValueJob, IntentRetriesUpdated, v)
}

// resolveIncident menyelesaikan incident lalu menjalankan ulang langkah yang gagal: job kembali ke
// antrean aktivasi (lewat Apply), atau perilaku aktivasi elemen dijalankan ulang. Langkah ulang
// yang gagal lagi menghasilkan incident baru; command tetap sukses.
func (r *run) resolveIncident(c ResolveIncident) error {
	if r.st.Status != InstanceActive {
		return ErrInstanceNotActive
	}
	inc, ok := r.st.Incidents[c.IncidentKey]
	if !ok {
		return ErrIncidentAlreadyResolved
	}
	if err := r.emit(inc.Key, ValueIncident, IntentResolved, incidentValue(inc)); err != nil {
		return err
	}
	el, ok := r.st.Elements[inc.ElementInstanceKey]
	if !ok {
		return nil
	}
	if inc.JobKey != "" {
		job, ok := r.st.Jobs[inc.JobKey]
		if ok && job.State == JobFailed {
			// Retries belum dinaikkan: job tetap FAILED dan incident baru dibuat.
			return r.raiseJobIncident(el, IncidentJobNoRetries, inc.Message, job.Key)
		}
		return nil
	}
	if el.State != ElementActivating {
		return nil
	}
	r.queue = append(r.queue, work{kind: workResume, elemKey: el.Key})
	return r.loop()
}

// resume menjalankan ulang perilaku aktivasi elemen yang masih ACTIVATING.
func (r *run) resume(elemKey string) error {
	el, ok := r.st.Elements[elemKey]
	if !ok || el.State != ElementActivating {
		return fmt.Errorf("%w: resume element instance %s that is not activating", ErrInvalidRecord, elemKey)
	}
	node, err := r.node(el.ElementID)
	if err != nil {
		return err
	}
	return r.behave(el, node)
}

func (r *run) setVariables(c SetVariables) error {
	if r.st.Status != InstanceActive {
		return ErrInstanceNotActive
	}
	if c.ScopeKey != r.st.Key {
		if _, ok := r.st.Elements[c.ScopeKey]; !ok {
			return ErrScopeNotFound
		}
		if c.Local {
			return ErrLocalScopeNotSupported
		}
	}
	// Hanya ada scope process instance sampai Fase 7: propagasi ke atas sama dengan menulis ke root.
	return r.mergeVariables(c.Variables)
}
