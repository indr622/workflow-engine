import { useState } from 'react';
import { Check } from 'lucide-react';
import { Badge, Button, cn, Textarea } from '@workflow/ui';
import { useInstanceUiStore } from '../application/instance.store';
import { useCompleteJob } from '../application/process-instance.queries';
import { isOpenJob, type Job } from '../domain/process-instance';
import { parseVariablesInput } from '../domain/variables-input';

export interface JobsPanelProps {
  instanceKey: string;
  jobs: Job[];
}

/** JobsPanel: daftar job terbuka dengan aksi "Complete" manual (pengganti worker di Fase 2). */
export function JobsPanel({ instanceKey, jobs }: JobsPanelProps) {
  const open = jobs.filter(isOpenJob);
  if (open.length === 0) {
    return <p className="text-sm text-muted-foreground">No open jobs.</p>;
  }
  return (
    <ul className="space-y-2 text-sm">
      {open.map((job) => (
        <JobRow key={job.key} instanceKey={instanceKey} job={job} />
      ))}
    </ul>
  );
}

function JobRow({ instanceKey, job }: { instanceKey: string; job: Job }) {
  const [editing, setEditing] = useState(false);
  const [variables, setVariables] = useState('{}');
  const [variablesError, setVariablesError] = useState<string | null>(null);
  const completeJob = useCompleteJob(instanceKey);
  const selected = useInstanceUiStore((s) => s.selectedElementId);
  const selectElement = useInstanceUiStore((s) => s.selectElement);

  const complete = () => {
    const parsed = parseVariablesInput(editing ? variables : '');
    if (!parsed.ok) {
      setVariablesError(parsed.error);
      return;
    }
    setVariablesError(null);
    completeJob.mutate({ jobKey: job.key, variables: parsed.json });
  };

  return (
    <li className={cn('space-y-2 rounded-md border p-2', selected === job.elementId && 'border-ring bg-accent')}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{job.type}</span>
        <button type="button" onClick={() => selectElement(job.elementId)}>
          <Badge variant="outline">{job.elementId}</Badge>
        </button>
        <span className="text-xs text-muted-foreground">retries {job.retries}</span>
        <span className="ml-auto flex gap-2">
          <Button variant="ghost" size="sm" onClick={() => setEditing((v) => !v)}>
            {editing ? 'No variables' : 'With variables'}
          </Button>
          <Button size="sm" disabled={completeJob.isPending} onClick={complete}>
            <Check /> {completeJob.isPending ? 'Completing…' : 'Complete'}
          </Button>
        </span>
      </div>
      <div className="font-mono text-xs text-muted-foreground">job {job.key}</div>
      {editing && (
        <Textarea
          value={variables}
          onChange={(e) => {
            setVariables(e.target.value);
            if (variablesError) setVariablesError(null);
          }}
          rows={4}
          spellCheck={false}
          className="font-mono text-xs"
          aria-label={`Variables for job ${job.key}`}
          aria-invalid={variablesError !== null}
        />
      )}
      {variablesError && <p className="text-destructive">{variablesError}</p>}
      {completeJob.isError && <p className="text-destructive">{completeJob.error.message}</p>}
    </li>
  );
}
