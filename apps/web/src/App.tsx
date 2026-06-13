import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Ledger } from "./Ledger";

// Authentication is handled by Authelia + Traefik at the proxy layer. By the
// time a request reaches this SPA the user is already verified — no auth gate
// or token management needed here.
const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Ledger />
    </QueryClientProvider>
  );
}
