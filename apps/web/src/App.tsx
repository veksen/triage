import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BoardView } from "./components/BoardView";

const queryClient = new QueryClient();

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BoardView />
    </QueryClientProvider>
  );
}
