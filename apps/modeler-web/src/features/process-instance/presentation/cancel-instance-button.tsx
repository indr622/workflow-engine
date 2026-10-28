import { Ban } from 'lucide-react';
import { Button } from '@workflow/ui';
import { useCancelInstance } from '../application/process-instance.queries';

/** CancelInstanceButton membatalkan instance aktif (state menjadi terminated). */
export function CancelInstanceButton({ instanceKey, disabled }: { instanceKey: string; disabled?: boolean }) {
  const cancel = useCancelInstance();
  return (
    <span className="inline-flex items-center gap-2">
      <Button
        variant="destructive"
        size="sm"
        disabled={disabled || cancel.isPending}
        onClick={() => {
          if (window.confirm('Cancel this process instance? Active tokens and open jobs are terminated.')) {
            cancel.mutate(instanceKey);
          }
        }}
      >
        <Ban /> Cancel
      </Button>
      {cancel.isError && <span className="text-xs text-destructive">{cancel.error.message}</span>}
    </span>
  );
}
