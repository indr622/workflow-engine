import { create } from 'zustand';

/**
 * State klien fitur process-instance: elemen diagram yang dipilih di viewer.
 * Data dari engine (instance, token, job) tidak disimpan di sini, tetapi di TanStack Query.
 */
interface InstanceUiState {
  selectedElementId: string | null;
  /** Memilih elemen; memilih elemen yang sama lagi membatalkan pilihan. */
  toggleElement: (elementId: string) => void;
  selectElement: (elementId: string | null) => void;
  clear: () => void;
}

export const useInstanceUiStore = create<InstanceUiState>()((set) => ({
  selectedElementId: null,
  toggleElement: (elementId) =>
    set((s) => ({ selectedElementId: s.selectedElementId === elementId ? null : elementId })),
  selectElement: (elementId) => set({ selectedElementId: elementId }),
  clear: () => set({ selectedElementId: null }),
}));
