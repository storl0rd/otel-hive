import { useEffect } from "react";
import {
  BrowserRouter as Router,
  Routes,
  Route,
  Navigate,
  useNavigate,
  useLocation,
} from "react-router-dom";

import Layout from "./layout";
import AgentsPage from "./pages/Agents";
import ConfigsPage from "./pages/Configs";
import GroupsPage from "./pages/Groups";
import TopologyPage from "./pages/Topology";
import LoginPage from "./pages/Login";
import SetupPage from "./pages/Setup";
import ApiKeysPage from "./pages/ApiKeys";
import GitSourcesPage from "./pages/GitSources";

import "./App.css";
import { ThemeProvider } from "@/components/ThemeProvider";
import { SWRProvider } from "@/lib/swr-provider";
import { ApiProvider } from "@/providers/ApiProvider";
import { AuthProvider, useAuth } from "@/hooks/useAuth";
import { registerRefreshFn } from "@/api/base";

// ─── RefreshBridge ─────────────────────────────────────────────────────────────
// Connects the auth context's refreshTokens fn to the API base layer so that
// any 401 response can trigger a silent refresh without circular imports.

function RefreshBridge() {
  const { refreshTokens } = useAuth();
  useEffect(() => {
    registerRefreshFn(refreshTokens);
  }, [refreshTokens]);
  return null;
}

// ─── RequireAuth ───────────────────────────────────────────────────────────────
// Wraps protected routes. Redirects based on auth status.

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { status } = useAuth();
  const location = useLocation();

  if (status === "loading") {
    // Render nothing while the initial auth check runs
    return null;
  }

  if (status === "setup_required") {
    return <Navigate to="/setup" replace />;
  }

  if (status === "unauthenticated") {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />;
  }

  return <>{children}</>;
}

// ─── RedirectIfAuthed ──────────────────────────────────────────────────────────
// Used on /login and /setup — redirects to the app if already authenticated.

function RedirectIfAuthed({ children }: { children: React.ReactNode }) {
  const { status } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (status === "authenticated") {
      navigate("/agents", { replace: true });
    }
  }, [status, navigate]);

  if (status === "loading") return null;
  return <>{children}</>;
}

// ─── App ──────────────────────────────────────────────────────────────────────

function App() {
  return (
    <ThemeProvider defaultTheme="system">
      <SWRProvider>
        <ApiProvider>
          <Router>
            <AuthProvider>
              <RefreshBridge />
              <Routes>
                {/* Public — unauthenticated only */}
                <Route
                  path="/login"
                  element={
                    <RedirectIfAuthed>
                      <LoginPage />
                    </RedirectIfAuthed>
                  }
                />
                <Route
                  path="/setup"
                  element={
                    <RedirectIfAuthed>
                      <SetupPage />
                    </RedirectIfAuthed>
                  }
                />

                {/* Protected — requires auth */}
                <Route
                  element={
                    <RequireAuth>
                      <Layout />
                    </RequireAuth>
                  }
                >
                  <Route path="/" element={<AgentsPage />} />
                  <Route path="/agents" element={<AgentsPage />} />
                  <Route path="/groups" element={<GroupsPage />} />
                  <Route path="/configs" element={<ConfigsPage />} />
                  <Route
                    path="/configs/new"
                    element={<ConfigsPage mode="create" />}
                  />
                  <Route
                    path="/configs/:configId/edit"
                    element={<ConfigsPage mode="edit" />}
                  />
                  <Route path="/topology" element={<TopologyPage />} />
                  <Route path="/git-sources" element={<GitSourcesPage />} />
                  <Route path="/api-keys" element={<ApiKeysPage />} />
                </Route>
              </Routes>
            </AuthProvider>
          </Router>
        </ApiProvider>
      </SWRProvider>
    </ThemeProvider>
  );
}

export default App;
