import {
  CheckCircle2,
  Clock,
  FolderGit2,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
  XCircle,
} from "lucide-react";
import { useState } from "react";
import useSWR from "swr";

import {
  createGitSource,
  deleteGitSource,
  listGitSources,
  triggerSync,
  updateGitSource,
} from "@/api/gitsync";
import type { CreateGitSourceRequest, GitSource, ProviderType } from "@/api/gitsync";
import { PageTable } from "@/components/shared/PageTable";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { TableCell } from "@/components/ui/table";

// ─── Helpers ──────────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  if (status === "success")
    return (
      <Badge variant="default" className="gap-1 bg-green-500 hover:bg-green-500/80">
        <CheckCircle2 className="h-3 w-3" />
        synced
      </Badge>
    );
  if (status === "failed")
    return (
      <Badge variant="destructive" className="gap-1">
        <XCircle className="h-3 w-3" />
        failed
      </Badge>
    );
  return (
    <Badge variant="secondary" className="gap-1">
      <Clock className="h-3 w-3" />
      {status || "pending"}
    </Badge>
  );
}

function relativeTime(ts?: string | null): string {
  if (!ts) return "—";
  const diff = Math.floor((Date.now() - new Date(ts).getTime()) / 1000);
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

// ─── Form defaults ────────────────────────────────────────────────────────────

const emptyForm = (): CreateGitSourceRequest => ({
  name: "",
  repo_url: "",
  token: "",
  branch: "main",
  config_root: "configs",
  provider: "github",
  poll_interval_seconds: 300,
  webhook_secret: "",
});

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function GitSourcesPage() {
  const [refreshing, setRefreshing] = useState(false);
  const [syncing, setSyncing] = useState<string | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<GitSource | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<GitSource | null>(null);
  const [form, setForm] = useState<CreateGitSourceRequest>(emptyForm());
  const [formError, setFormError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const { data, error, mutate } = useSWR("git-sources", listGitSources, {
    refreshInterval: 30000,
  });

  const sources = data?.git_sources ?? [];

  // ─── Handlers ───────────────────────────────────────────────────────────────

  const handleRefresh = async () => {
    setRefreshing(true);
    await mutate();
    setRefreshing(false);
  };

  const openCreate = () => {
    setEditTarget(null);
    setForm(emptyForm());
    setFormError(null);
    setSheetOpen(true);
  };

  const openEdit = (gs: GitSource) => {
    setEditTarget(gs);
    setForm({
      name: gs.name,
      repo_url: gs.repo_url,
      token: "",           // never echoed back from server
      branch: gs.branch,
      config_root: gs.config_root,
      provider: gs.provider,
      poll_interval_seconds: gs.poll_interval_seconds,
      webhook_secret: "",  // never echoed back from server
    });
    setFormError(null);
    setSheetOpen(true);
  };

  const handleSave = async () => {
    setSaving(true);
    setFormError(null);
    try {
      if (editTarget) {
        await updateGitSource(editTarget.id, form);
      } else {
        await createGitSource(form);
      }
      setSheetOpen(false);
      await mutate();
    } catch (err: unknown) {
      setFormError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteGitSource(deleteTarget.id);
      setDeleteTarget(null);
      await mutate();
    } catch (err) {
      console.error("Delete failed:", err);
    }
  };

  const handleSyncNow = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setSyncing(id);
    try {
      await triggerSync(id);
      await mutate();
    } catch (err) {
      console.error("Sync failed:", err);
    } finally {
      setSyncing(null);
    }
  };

  // ─── Render ─────────────────────────────────────────────────────────────────

  if (error) {
    return (
      <div className="container mx-auto p-6">
        <div className="text-center">
          <h1 className="text-2xl font-bold text-red-600 mb-4">Error Loading Git Sources</h1>
          <p className="text-muted-foreground">{error.message}</p>
          <Button onClick={handleRefresh} className="mt-4">
            <RefreshCw className="h-4 w-4 mr-2" />
            Retry
          </Button>
        </div>
      </div>
    );
  }

  return (
    <>
      <PageTable
        pageTitle="Git Sources"
        pageDescription="Sync collector configs from git repositories via polling or webhook"
        pageActions={[
          {
            label: "Refresh",
            icon: RefreshCw,
            onClick: handleRefresh,
            disabled: refreshing,
            variant: "ghost" as const,
          },
          {
            label: "Add Source",
            icon: Plus,
            onClick: openCreate,
            variant: "default" as const,
          },
        ]}
        cardTitle={`Git Sources (${sources.length})`}
        cardDescription="Git repositories polled for collector configuration files"
        columns={[
          { header: "Name", key: "name" },
          { header: "Provider", key: "provider" },
          { header: "Branch", key: "branch" },
          { header: "Config Root", key: "config_root" },
          { header: "Status", key: "status" },
          { header: "Last Sync", key: "last_sync" },
          { header: "Actions", key: "actions" },
        ]}
        data={sources}
        getRowKey={(gs: GitSource) => gs.id}
        renderRow={(gs: GitSource) => (
          <>
            <TableCell className="font-medium">{gs.name}</TableCell>
            <TableCell>
              <Badge variant="outline">{gs.provider}</Badge>
            </TableCell>
            <TableCell className="font-mono text-xs">{gs.branch || "main"}</TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">
              {gs.config_root || "configs"}
            </TableCell>
            <TableCell>
              <StatusBadge status={gs.last_sync_status} />
              {gs.last_sync_status === "failed" && gs.last_sync_error && (
                <p
                  className="mt-1 text-[11px] text-destructive truncate max-w-[180px]"
                  title={gs.last_sync_error}
                >
                  {gs.last_sync_error}
                </p>
              )}
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {relativeTime(gs.last_sync_at)}
            </TableCell>
            <TableCell>
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={(e) => handleSyncNow(gs.id, e)}
                  disabled={syncing === gs.id}
                  title="Sync Now"
                >
                  {syncing === gs.id ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <RefreshCw className="h-4 w-4" />
                  )}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={(e) => { e.stopPropagation(); openEdit(gs); }}
                  title="Edit"
                >
                  <Pencil className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={(e) => { e.stopPropagation(); setDeleteTarget(gs); }}
                  title="Delete"
                  className="text-red-600 hover:text-red-700 hover:bg-red-50 dark:text-red-400 dark:hover:text-red-300 dark:hover:bg-red-950/30"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </TableCell>
          </>
        )}
        emptyState={{
          icon: FolderGit2,
          title: "No Git Sources",
          description: "Add a git repository to start syncing collector configs.",
          action: { label: "Add Source", onClick: openCreate },
        }}
      />

      {/* Create / Edit Sheet */}
      <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
        <SheetContent className="w-full sm:max-w-lg overflow-y-auto">
          <SheetHeader>
            <SheetTitle>{editTarget ? "Edit Git Source" : "Add Git Source"}</SheetTitle>
            <SheetDescription>
              {editTarget
                ? "Update repository settings. Leave token / webhook secret blank to keep existing values."
                : "Connect a git repository to automatically sync collector configs."}
            </SheetDescription>
          </SheetHeader>

          <div className="grid gap-4 py-4">
            <div className="grid gap-1.5">
              <Label htmlFor="gs-name">Name *</Label>
              <Input
                id="gs-name"
                placeholder="production-configs"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="gs-provider">Provider *</Label>
              <Select
                value={form.provider}
                onValueChange={(v) => setForm({ ...form, provider: v as ProviderType })}
              >
                <SelectTrigger id="gs-provider">
                  <SelectValue placeholder="Select provider" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="github">GitHub</SelectItem>
                  <SelectItem value="gitlab">GitLab</SelectItem>
                  <SelectItem value="gitea">Gitea</SelectItem>
                  <SelectItem value="http">Generic HTTP</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="gs-url">Repository URL *</Label>
              <Input
                id="gs-url"
                placeholder="https://github.com/org/otel-configs"
                value={form.repo_url}
                onChange={(e) => setForm({ ...form, repo_url: e.target.value })}
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="gs-token">
                Access Token
                {editTarget?.has_token && (
                  <span className="ml-1 text-xs text-muted-foreground">
                    (blank = keep existing)
                  </span>
                )}
              </Label>
              <Input
                id="gs-token"
                type="password"
                placeholder={
                  editTarget?.has_token
                    ? "••••••••"
                    : "ghp_... (leave blank for public repos)"
                }
                value={form.token}
                onChange={(e) => setForm({ ...form, token: e.target.value })}
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="gs-branch">Branch</Label>
                <Input
                  id="gs-branch"
                  placeholder="main"
                  value={form.branch}
                  onChange={(e) => setForm({ ...form, branch: e.target.value })}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="gs-root">Config Root</Label>
                <Input
                  id="gs-root"
                  placeholder="configs"
                  value={form.config_root}
                  onChange={(e) => setForm({ ...form, config_root: e.target.value })}
                />
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="gs-poll">Poll Interval (seconds)</Label>
              <Input
                id="gs-poll"
                type="number"
                min={30}
                placeholder="300"
                value={form.poll_interval_seconds}
                onChange={(e) =>
                  setForm({
                    ...form,
                    poll_interval_seconds: parseInt(e.target.value) || 300,
                  })
                }
              />
              <p className="text-xs text-muted-foreground">
                Minimum 30s. Use webhooks for instant delivery.
              </p>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="gs-webhook">
                Webhook Secret
                {editTarget?.has_webhook_secret && (
                  <span className="ml-1 text-xs text-muted-foreground">
                    (blank = keep existing)
                  </span>
                )}
              </Label>
              <Input
                id="gs-webhook"
                type="password"
                placeholder={
                  editTarget?.has_webhook_secret
                    ? "••••••••"
                    : "Optional HMAC secret for webhook validation"
                }
                value={form.webhook_secret}
                onChange={(e) => setForm({ ...form, webhook_secret: e.target.value })}
              />
              {editTarget && (
                <p className="text-xs text-muted-foreground font-mono break-all">
                  Webhook: /api/webhook/git/{editTarget.id}
                </p>
              )}
            </div>

            {formError && (
              <p className="text-sm text-destructive rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2">
                {formError}
              </p>
            )}
          </div>

          <SheetFooter>
            <Button variant="outline" onClick={() => setSheetOpen(false)} disabled={saving}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={saving}>
              {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {editTarget ? "Save Changes" : "Add Source"}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* Delete Confirmation */}
      <Dialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Git Source</DialogTitle>
            <DialogDescription>
              Remove <strong>{deleteTarget?.name}</strong>? This stops background polling and
              removes sync history. Agent configs already pushed are not affected.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
