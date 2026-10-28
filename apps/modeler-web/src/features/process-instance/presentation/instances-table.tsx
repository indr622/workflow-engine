import { useMemo, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { createColumnHelper, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { Badge, Button, Input, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@workflow/ui';
import { useProcessInstances } from '../application/process-instance.queries';
import type { ProcessInstance } from '../domain/process-instance';
import { InstanceStateBadge } from './instance-state-badge';

const columns = createColumnHelper<ProcessInstance>();

export interface InstancesTableProps {
  /** Filter bpmnProcessId (dari search param route). */
  bpmnProcessId: string;
  onBpmnProcessIdChange: (value: string) => void;
}

/** InstancesTable: daftar process instance dengan filter process id, incident, dan pagination. */
export function InstancesTable({ bpmnProcessId, onBpmnProcessIdChange }: InstancesTableProps) {
  const [onlyWithIncident, setOnlyWithIncident] = useState(false);
  const [pageToken, setPageToken] = useState('');
  const [history, setHistory] = useState<string[]>([]);

  const params = useMemo(
    () => ({ bpmnProcessId: bpmnProcessId.trim(), onlyWithIncident, pageSize: 20, pageToken }),
    [bpmnProcessId, onlyWithIncident, pageToken],
  );
  const { data, isPending, isError, error } = useProcessInstances(params);

  const table = useReactTable({
    data: data?.items ?? [],
    columns: [
      columns.accessor('key', {
        header: 'Instance key',
        cell: (c) => (
          <Link
            to="/instances/$key"
            params={{ key: c.getValue() }}
            className="font-mono text-xs underline-offset-4 hover:underline"
          >
            {c.getValue()}
          </Link>
        ),
      }),
      columns.accessor('bpmnProcessId', {
        header: 'Process id',
        cell: (c) => <span className="font-medium">{c.getValue()}</span>,
      }),
      columns.accessor('version', {
        header: 'Version',
        cell: (c) => <Badge variant="secondary">v{c.getValue()}</Badge>,
      }),
      columns.display({ id: 'state', header: 'State', cell: (c) => <InstanceStateBadge instance={c.row.original} /> }),
      columns.accessor('startTime', { header: 'Started', cell: (c) => c.getValue()?.toLocaleString() ?? '—' }),
      columns.accessor('endTime', { header: 'Ended', cell: (c) => c.getValue()?.toLocaleString() ?? '—' }),
    ],
    getCoreRowModel: getCoreRowModel(),
  });

  const resetPaging = () => {
    setPageToken('');
    setHistory([]);
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          placeholder="Filter by process id"
          value={bpmnProcessId}
          onChange={(e) => {
            onBpmnProcessIdChange(e.target.value);
            resetPaging();
          }}
          className="max-w-xs"
        />
        <Button
          variant={onlyWithIncident ? 'default' : 'outline'}
          size="sm"
          onClick={() => {
            setOnlyWithIncident((v) => !v);
            resetPaging();
          }}
        >
          Only with incident: {onlyWithIncident ? 'on' : 'off'}
        </Button>
      </div>
      {isError && <p className="text-sm text-destructive">{error.message}</p>}
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((hg) => (
            <TableRow key={hg.id}>
              {hg.headers.map((h) => (
                <TableHead key={h.id}>{flexRender(h.column.columnDef.header, h.getContext())}</TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {isPending && (
            <TableRow>
              <TableCell colSpan={6} className="text-muted-foreground">
                Loading…
              </TableCell>
            </TableRow>
          )}
          {!isPending && table.getRowModel().rows.length === 0 && (
            <TableRow>
              <TableCell colSpan={6} className="text-muted-foreground">
                No process instances yet. Start one from the definitions list.
              </TableCell>
            </TableRow>
          )}
          {table.getRowModel().rows.map((row) => (
            <TableRow key={row.id}>
              {row.getVisibleCells().map((cell) => (
                <TableCell key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={history.length === 0}
          onClick={() => {
            const prev = [...history];
            setPageToken(prev.pop() ?? '');
            setHistory(prev);
          }}
        >
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!data?.nextPageToken}
          onClick={() => {
            setHistory((h) => [...h, pageToken]);
            setPageToken(data?.nextPageToken ?? '');
          }}
        >
          Next
        </Button>
      </div>
    </div>
  );
}
