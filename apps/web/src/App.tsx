import { AppProvider } from "./state/appState";
import { Shell } from "./ui/Shell";

export function App() {
  return (
    <AppProvider>
      <Shell />
    </AppProvider>
  );
}
