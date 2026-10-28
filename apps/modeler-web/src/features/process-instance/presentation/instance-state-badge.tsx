import { Badge } from '@workflow/ui';
import type { ProcessInstance } from '../domain/process-instance';

/** InstanceStateBadge menampilkan state instance dan tanda incident. */
export function InstanceStateBadge({ instance }: { instance: Pick<ProcessInstance, 'state' | 'hasIncident'> }) {
  return (
    <span className="inline-flex gap-1">
      {instance.state === 'active' && <Badge variant="success">active</Badge>}
      {instance.state === 'completed' && <Badge variant="secondary">completed</Badge>}
      {instance.state === 'terminated' && <Badge variant="outline">terminated</Badge>}
      {instance.state === 'unspecified' && <Badge variant="outline">unknown</Badge>}
      {instance.hasIncident && <Badge variant="destructive">incident</Badge>}
    </span>
  );
}
