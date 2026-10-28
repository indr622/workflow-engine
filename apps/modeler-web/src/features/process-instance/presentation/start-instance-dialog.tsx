import { useState, type ReactNode } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { Play } from 'lucide-react';
import {
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Input,
  Textarea,
} from '@workflow/ui';
import { useCreateInstance } from '../application/process-instance.queries';
import { parseVariablesInput } from '../domain/variables-input';

export interface StartInstanceDialogProps {
  /** Key definition yang dipilih awal; masih bisa diganti di dialog. */
  definitionKey: string;
  bpmnProcessId?: string;
  version?: number;
  /** Tombol pembuka dialog; default tombol kecil "Start". */
  trigger?: ReactNode;
}

/**
 * StartInstanceDialog menjalankan process instance baru: memilih key definition, mengisi variabel
 * JSON (divalidasi sebelum dikirim), lalu membuka halaman instance setelah berhasil dibuat.
 */
export function StartInstanceDialog({ definitionKey, bpmnProcessId, version, trigger }: StartInstanceDialogProps) {
  const [open, setOpen] = useState(false);
  const [key, setKey] = useState(definitionKey);
  const [variables, setVariables] = useState('{}');
  const [variablesError, setVariablesError] = useState<string | null>(null);
  const createInstance = useCreateInstance();
  const navigate = useNavigate();

  const onOpenChange = (next: boolean) => {
    setOpen(next);
    if (next) {
      setKey(definitionKey);
      setVariablesError(null);
      createInstance.reset();
    }
  };

  const submit = () => {
    const parsed = parseVariablesInput(variables);
    if (!parsed.ok) {
      setVariablesError(parsed.error);
      return;
    }
    setVariablesError(null);
    createInstance.mutate(
      { definition: { kind: 'key', definitionKey: key.trim() }, variables: parsed.json },
      {
        onSuccess: (created) => {
          setOpen(false);
          void navigate({ to: '/instances/$key', params: { key: created.key } });
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger asChild>
        {trigger ?? (
          <Button variant="outline" size="sm">
            <Play /> Start
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Start process instance</DialogTitle>
          <DialogDescription>
            {bpmnProcessId ? `${bpmnProcessId}${version ? ` v${version}` : ''}` : 'Run a deployed process definition'}
          </DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <label className="grid gap-1.5 text-sm font-medium">
            Process definition key
            <Input
              value={key}
              onChange={(e) => setKey(e.target.value)}
              className="font-mono text-xs"
              required
              aria-invalid={key.trim() === ''}
            />
          </label>
          <label className="grid gap-1.5 text-sm font-medium">
            Variables (JSON object)
            <Textarea
              value={variables}
              onChange={(e) => {
                setVariables(e.target.value);
                if (variablesError) setVariablesError(null);
              }}
              rows={8}
              spellCheck={false}
              className="min-h-32 font-mono text-xs"
              aria-invalid={variablesError !== null}
            />
          </label>
          {variablesError && <p className="text-sm text-destructive">{variablesError}</p>}
          {createInstance.isError && <p className="text-sm text-destructive">{createInstance.error.message}</p>}
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit" disabled={createInstance.isPending || key.trim() === ''}>
              <Play /> {createInstance.isPending ? 'Starting…' : 'Start instance'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
