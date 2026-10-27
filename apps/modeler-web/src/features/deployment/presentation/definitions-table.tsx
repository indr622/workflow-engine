import { useMemo, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { createColumnHelper, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { Badge, Button, Input, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@workflow/ui';
import { useProcessDefinitions } from '../application/deployment.queries';
import type { ProcessDefinition } from '../domain/deployment';

const columns = createColumnHelper<ProcessDefinition>();

/** DefinitionsTable: daftar process definition dengan filter, toggle latest-only, dan pagination. */
export function DefinitionsTable() {
  const [filter, setFilter] = useState('');
  const [latestOnly, setLatestOnly] = useState(true);
  const [pageToken, setPageToken] = useState('');
  const [history, setHistory] = useState<string[]>([]);

  const params = useMemo(
    () => ({ bpmnProcessId: filter.trim(), latestOnly, pageSize: 20, pageToken }),
    [filter, latestOnly, pageToken],
  );
  const { data, isPending, isError, error } = useProcessDefinitions(params);

  const table = useReactTable({
    data: data?.items ?? [],
    columns: [
      columns.accessor('bpmnProcessId', {
        header: 'Process id',
        cell: (c) => (
          <Link
            to="/definitions/$key"
            params={{ key: c.row.original.key }}
            className="font-medium underline-offset-4 hover:underline"
          >
            {c.getValue()}
          </Link>
        ),
      }),
      columns.accessor('name', { header: 'Name' }),
      columns.accessor('version', {
        header: 'Version',
        cell: (c) => <Badge variant="secondary">v{c.getValue()}</Badge>,
      }),
      columns.accessor('versionTag', { header: 'Tag', cell: (c) => c.getValue() || '—' }),
      columns.accessor('resourceName', { header: 'Resource' }),
      columns.accessor('key', {
        header: 'Key',
        cell: (c) => <span className="font-mono text-xs">{c.getValue()}</span>,
      }),
      columns.accessor('createdAt', { header: 'Deployed', cell: (c) => c.getValue().toLocaleString() }),
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
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
            resetPaging();
          }}
          className="max-w-xs"
        />
        <Button
          variant={latestOnly ? 'default' : 'outline'}
          size="sm"
          onClick={() => {
            setLatestOnly((v) => !v);
            resetPaging();
          }}
        >
          Latest only: {latestOnly ? 'on' : 'off'}
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
              <TableCell colSpan={7} className="text-muted-foreground">
                Loading…
              </TableCell>
            </TableRow>
          )}
          {!isPending && table.getRowModel().rows.length === 0 && (
            <TableRow>
              <TableCell colSpan={7} className="text-muted-foreground">
                No process definitions deployed yet.
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
