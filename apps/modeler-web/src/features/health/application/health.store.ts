import { create } from 'zustand';

/** State klien fitur health. Data dari engine tidak disimpan di sini, tetapi di TanStack Query. */
interface HealthUiState {
  autoRefresh: boolean;
  toggleAutoRefresh: () => void;
}

export const useHealthUiStore = create<HealthUiState>()((set) => ({
  autoRefresh: true,
  toggleAutoRefresh: () => set((s) => ({ autoRefresh: !s.autoRefresh })),
}));
