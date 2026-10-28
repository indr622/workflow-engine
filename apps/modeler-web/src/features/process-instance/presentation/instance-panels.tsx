import { CircleX } from 'lucide-react';
import { Badge, cn, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@workflow/ui';
import { useInstanceUiStore } from '../application/instance.store';
import { isActiveElementState, type ElementInstance, type Incident, type Variable } from '../domain/process-instance';
import { formatJsonValue } from '../domain/variables-input';

/** IncidentsPanel: incident instance; klik memilih elemennya di diagram. */
export function IncidentsPanel({ incidents }: { incidents: Incident[] }) {
  const selectElement = useInstanceUiStore((s) => s.selectElement);
  if (incidents.length === 0) {
    return <p className="text-sm text-muted-foreground">No incidents.</p>;
  }
  return (
    <ul className="space-y-1 text-sm">
      {incidents.map((incident) => (
        <li key={incident.key}>
          <button
            type="button"
            onClick={() => selectElement(incident.elementId)}
            className="flex w-full items-start gap-2 rounded-md px-2 py-1 text-left hover:bg-accent"
          >
            <CircleX
              className={cn('mt-0.5 size-4 shrink-0', incident.resolved ? 'text-muted-foreground' : 'text-destructive')}
            />
            <span className="min-w-0 flex-1">
              <span className="font-medium">{incident.errorType}</span>
              <Badge variant="outline" className="ml-2">
                {incident.elementId}
              </Badge>
              {incident.resolved && (
                <Badge variant="secondary" className="ml-2">
                  resolved
                </Badge>
              )}
              <span className="block text-muted-foreground">{incident.errorMessage}</span>
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

/** VariablesPanel: variabel instance per scope; nilai ditampilkan sebagai JSON. */
export function VariablesPanel({ variables, rootScopeKey }: { variables: Variable[]; rootScopeKey: string }) {
  if (variables.length === 0) {
    return <p className="text-sm text-muted-foreground">No variables.</p>;
  }
  const sorted = [...variables].sort((a, b) => a.name.localeCompare(b.name) || a.scopeKey.localeCompare(b.scopeKey));
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Value</TableHead>
          <TableHead>Scope</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {sorted.map((v) => (
          <TableRow key={`${v.scopeKey}|${v.name}`}>
            <TableCell className="font-medium">{v.name}</TableCell>
            <TableCell className="max-w-xs truncate font-mono text-xs" title={v.value}>
              {formatJsonValue(v.value)}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {v.scopeKey === rootScopeKey ? 'process' : <span className="font-mono">{v.scopeKey}</span>}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

/** ElementInstancesPanel: riwayat element instance; baris elemen terpilih disorot. */
export function ElementInstancesPanel({ elementInstances }: { elementInstances: ElementInstance[] }) {
  const selected = useInstanceUiStore((s) => s.selectedElementId);
  const toggleElement = useInstanceUiStore((s) => s.toggleElement);
  if (elementInstances.length === 0) {
    return <p className="text-sm text-muted-foreground">No element instances.</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Element</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>State</TableHead>
          <TableHead>Started</TableHead>
          <TableHead>Ended</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {elementInstances.map((ei) => (
          <TableRow
            key={ei.key}
            onClick={() => toggleElement(ei.elementId)}
            className={cn('cursor-pointer', selected === ei.elementId && 'bg-accent')}
          >
            <TableCell className="font-medium">{ei.elementId}</TableCell>
            <TableCell className="text-xs text-muted-foreground">{ei.elementType}</TableCell>
            <TableCell>
              <Badge
                variant={
                  ei.incidentKey && isActiveElementState(ei.state)
                    ? 'destructive'
                    : isActiveElementState(ei.state)
                      ? 'success'
                      : 'secondary'
                }
              >
                {ei.state}
              </Badge>
            </TableCell>
            <TableCell className="text-xs">{ei.startTime?.toLocaleTimeString() ?? '—'}</TableCell>
            <TableCell className="text-xs">{ei.endTime?.toLocaleTimeString() ?? '—'}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
