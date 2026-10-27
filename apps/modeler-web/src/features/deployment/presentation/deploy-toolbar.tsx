import { CheckCircle2, Rocket, ShieldCheck } from 'lucide-react';
import { Badge, Button } from '@workflow/ui';
import { useDeployResource, useValidateResource } from '../application/deployment.queries';
import { useDeploymentUiStore } from '../application/deployment.store';
import { blockingIssues, DeploymentRejectedError } from '../domain/deployment';

export interface DeployToolbarProps {
  /** Mengambil XML diagram saat ini dari kanvas. */
  getXml: () => Promise<string>;
  resourceName?: string;
}

/** DeployToolbar: tombol validasi dan deploy beserta ringkasan hasil terakhir. */
export function DeployToolbar({ getXml, resourceName = 'diagram.bpmn' }: DeployToolbarProps) {
  const deploy = useDeployResource();
  const validate = useValidateResource();
  const issues = useDeploymentUiStore((s) => s.issues);
  const lastDeploymentKey = useDeploymentUiStore((s) => s.lastDeploymentKey);
  const errors = blockingIssues(issues).length;
  const busy = deploy.isPending || validate.isPending;

  const run = async (action: typeof deploy | typeof validate) => {
    const content = await getXml();
    action.mutate([{ name: resourceName, content }]);
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="outline" size="sm" disabled={busy} onClick={() => run(validate)}>
        <ShieldCheck /> Validate
      </Button>
      <Button size="sm" disabled={busy} onClick={() => run(deploy)}>
        <Rocket /> Deploy
      </Button>
      {deploy.isSuccess && (
        <Badge variant="success">
          <CheckCircle2 /> deployed{' '}
          {deploy.data.definitions
            .map((d) => `${d.bpmnProcessId} v${d.version}${d.duplicate ? ' (unchanged)' : ''}`)
            .join(', ')}
        </Badge>
      )}
      {deploy.isError && deploy.error instanceof DeploymentRejectedError && (
        <Badge variant="destructive">rejected: {errors} error(s)</Badge>
      )}
      {deploy.isError && !(deploy.error instanceof DeploymentRejectedError) && (
        <Badge variant="destructive">deploy failed: {deploy.error.message}</Badge>
      )}
      {validate.isSuccess && !deploy.isSuccess && (
        <Badge variant={validate.data.valid ? 'success' : 'destructive'}>
          {validate.data.valid ? 'valid' : `${errors} error(s)`}
        </Badge>
      )}
      {lastDeploymentKey && <span className="text-xs text-muted-foreground">deployment {lastDeploymentKey}</span>}
    </div>
  );
}
