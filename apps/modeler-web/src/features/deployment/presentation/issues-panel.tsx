import { AlertTriangle, CircleX } from 'lucide-react';
import { Badge, cn } from '@workflow/ui';
import { useDeploymentUiStore } from '../application/deployment.store';
import type { ValidationIssue } from '../domain/deployment';

/** IssuesPanel menampilkan issue validasi; klik memilih issue (kanvas menyorot elemennya). */
export function IssuesPanel() {
  const issues = useDeploymentUiStore((s) => s.issues);
  const selected = useDeploymentUiStore((s) => s.selectedIssue);
  const selectIssue = useDeploymentUiStore((s) => s.selectIssue);

  if (issues.length === 0) {
    return <p className="text-sm text-muted-foreground">No validation issues.</p>;
  }
  return (
    <ul className="max-h-64 space-y-1 overflow-y-auto text-sm">
      {issues.map((issue) => (
        <li key={issueKey(issue)}>
          <button
            type="button"
            onClick={() => selectIssue(issue)}
            className={cn(
              'flex w-full items-start gap-2 rounded-md px-2 py-1 text-left hover:bg-accent',
              selected && issueKey(selected) === issueKey(issue) && 'bg-accent',
            )}
          >
            {issue.severity === 'error' ? (
              <CircleX className="mt-0.5 size-4 shrink-0 text-destructive" />
            ) : (
              <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-500" />
            )}
            <span className="min-w-0 flex-1">
              <span className="font-medium">{issue.code}</span>
              {issue.elementId && (
                <Badge variant="outline" className="ml-2">
                  {issue.elementId}
                </Badge>
              )}
              <span className="block text-muted-foreground">{issue.message}</span>
            </span>
            {issue.line > 0 && (
              <span className="shrink-0 text-xs text-muted-foreground">
                {issue.line}:{issue.column}
              </span>
            )}
          </button>
        </li>
      ))}
    </ul>
  );
}

function issueKey(i: ValidationIssue): string {
  return `${i.code}|${i.elementId}|${i.line}|${i.column}`;
}
