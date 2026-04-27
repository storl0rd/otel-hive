import { ClipboardList, RefreshCw, ChevronLeft, ChevronRight } from "lucide-react";
import { useState } from "react";
import useSWR from "swr";

import { listAuditLog } from "@/api/auditlog";
import type { AuditEntry } from "@/api/auditlog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

// ─── Helpers ──────────────────────────────────────────────────────────────────

const EVENT_CATEGORIES: Record<string, string[]> = {
  "All events": [],
  "Auth": ["user.login", "user.logout", "user.setup", "api_key.created", "api_key.revoked"],
  "Configs": ["config.created", "config.updated", "config.deleted", "config.pushed"],
  "Agents": ["agent.restarted"],
  "Groups": ["group.created", "group.updated", "group.deleted"],
  "Git Sources": ["git_source.created", "git_source.updated", "git_source.deleted", "git_source.synced"],
};

const ALL_EVENT_TYPES = Object.values(EVENT_CATEGORIES).flat();

function eventBadgeVariant(event: string): "default" | "secondary" | "destructive" | "outline" {
  if (event.endsWith(".deleted") || event.endsWith(".revoked")) return "destructive";
  if (event.endsWith(".created") || event.endsWith(".setup")) return "default";
  if (event.endsWith(".pushed") || event.endsWith(".synced") || event.endsWith(".restarted")) return "secondary";
  return "outline";
}

function formatTimestamp(ts: string): string {
  return new Date(ts).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// ─── Page ─────────────────────────────────────────────────────────────────────

const PAGE_LIMIT = 50;

export default function AuditLogPage() {
  const [page, setPage] = useState(1);
  const [eventFilter, setEventFilter] = useState("");
  const [refreshing, setRefreshing] = useState(false);

  const swrKey = ["audit-log", page, eventFilter];
  const { data, error, mutate } = useSWR(
    swrKey,
    () => listAuditLog(page, PAGE_LIMIT, eventFilter || undefined),
    { refreshInterval: 60000 },
  );

  const entries: AuditEntry[] = data?.entries ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_LIMIT));

  const handleRefresh = async () => {
    setRefreshing(true);
    await mutate();
    setRefreshing(false);
  };

  const handleFilterChange = (val: string) => {
    setEventFilter(val === "all" ? "" : val);
    setPage(1);
  };

  if (error) {
    return (
      <div className="container mx-auto p-6 text-center">
        <h1 className="text-2xl font-bold text-red-600 mb-2">Error Loading Audit Log</h1>
        <p className="text-muted-foreground">{error.message}</p>
        <Button onClick={handleRefresh} className="mt-4">
          <RefreshCw className="h-4 w-4 mr-2" />
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto p-6 space-y-6">
      {/* Header */}
      <div className="flex justify-between items-center pb-4 border-b border-border">
        <div>
          <h1 className="text-3xl font-bold">Audit Log</h1>
          <p className="text-muted-foreground mt-1">
            Track all write operations performed by users and the system
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* Event type filter */}
          <Select onValueChange={handleFilterChange} defaultValue="all">
            <SelectTrigger className="w-44">
              <SelectValue placeholder="Filter by event" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All events</SelectItem>
              {ALL_EVENT_TYPES.map((et) => (
                <SelectItem key={et} value={et}>
                  {et}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button variant="ghost" onClick={handleRefresh} disabled={refreshing}>
            <RefreshCw className={`h-4 w-4 mr-2 ${refreshing ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </div>
      </div>

      {/* Table card */}
      <Card>
        <CardHeader>
          <div className="flex justify-between items-start">
            <div>
              <CardTitle>Events ({total.toLocaleString()})</CardTitle>
              <CardDescription>
                Newest first · Page {page} of {totalPages}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {entries.length === 0 && !error ? (
            <div className="text-center py-12">
              <ClipboardList className="h-12 w-12 text-muted-foreground/40 mx-auto mb-4" />
              <p className="text-muted-foreground">
                {eventFilter ? `No events matching "${eventFilter}"` : "No audit events yet"}
              </p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-44">Timestamp</TableHead>
                  <TableHead>Event</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>IP</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((e) => (
                  <TableRow key={e.id}>
                    <TableCell className="text-xs text-muted-foreground font-mono whitespace-nowrap">
                      {formatTimestamp(e.timestamp)}
                    </TableCell>
                    <TableCell>
                      <Badge variant={eventBadgeVariant(e.event_type)} className="font-mono text-xs">
                        {e.event_type}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm">
                      {e.actor_name ? (
                        <span className="font-medium">{e.actor_name}</span>
                      ) : (
                        <span className="text-muted-foreground italic">system</span>
                      )}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground font-mono">
                      {e.resource_type && (
                        <span>
                          {e.resource_type}
                          {e.resource_id ? `/${e.resource_id.slice(0, 8)}` : ""}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground font-mono">
                      {e.ip_address || "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-end gap-2 mt-4 pt-4 border-t">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
              >
                <ChevronLeft className="h-4 w-4" />
                Previous
              </Button>
              <span className="text-sm text-muted-foreground">
                {page} / {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
              >
                Next
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
