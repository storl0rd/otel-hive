import {
  Server,
  Users,
  FileText,
  GitBranch,
  FolderGit2,
  Sparkle,
  LogOut,
  KeyRound,
  ChevronUp,
} from "lucide-react";
import * as React from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";

import { ModeToggle } from "./mode-toggle";
import { useAuth } from "@/hooks/useAuth";

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarFooter,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Badge } from "@/components/ui/badge";

interface MenuItem {
  key: string;
  title: string;
  url: string;
  icon: React.ComponentType<{ className?: string }>;
}

const ROLE_VARIANT: Record<string, "default" | "secondary" | "outline"> = {
  admin: "default",
  operator: "secondary",
  viewer: "outline",
};

export function AppSidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const { state } = useSidebar();
  const { user, logout } = useAuth();

  const mainItems: MenuItem[] = [
    { key: "agents", title: "Agents", url: "/agents", icon: Server },
    { key: "topology", title: "Topology", url: "/topology", icon: GitBranch },
    { key: "groups", title: "Groups", url: "/groups", icon: Users },
    { key: "configs", title: "Configs", url: "/configs", icon: FileText },
    { key: "git-sources", title: "Git Sources", url: "/git-sources", icon: FolderGit2 },
  ];

  async function handleLogout() {
    await logout();
    navigate("/login", { replace: true });
  }

  return (
    <Sidebar collapsible="icon" className="border-r border-border">
      <SidebarHeader className="border-b border-border h-16 flex items-center justify-center relative">
        <SidebarMenu>
          <SidebarMenuItem>
            {state === "collapsed" ? (
              <div className="relative group">
                <div className="flex items-center justify-center h-8 w-8 rounded-md transition-colors group-hover:opacity-0">
                  <Sparkle className="h-4 w-4 text-primary" />
                </div>
                <SidebarTrigger className="absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity" />
              </div>
            ) : (
              <SidebarMenuButton asChild>
                <a href="/" className="flex items-center space-x-2">
                  <Sparkle className="h-4 w-4 text-primary" />
                  <span>OTel Hive</span>
                </a>
              </SidebarMenuButton>
            )}
          </SidebarMenuItem>
        </SidebarMenu>
        {state === "expanded" && (
          <SidebarTrigger className="absolute right-2 top-1/2 -translate-y-1/2 h-6 w-6" />
        )}
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Navigation</SidebarGroupLabel>
          <SidebarMenu>
            {mainItems.map((item) => {
              const isActive = location.pathname === item.url;
              return (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton
                    asChild
                    isActive={isActive}
                    tooltip={item.title}
                  >
                    <Link to={item.url} className="relative">
                      <item.icon />
                      {state === "expanded" && <span>{item.title}</span>}
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              );
            })}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter className="border-t border-border">
        <SidebarMenu>
          {/* Theme toggle */}
          <SidebarMenuItem>
            <ModeToggle iconOnly={state === "collapsed"} />
          </SidebarMenuItem>

          {/* User menu */}
          {user && (
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuButton
                    tooltip={user.username}
                    className="w-full"
                  >
                    <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary text-xs font-semibold uppercase">
                      {user.username.charAt(0)}
                    </div>
                    {state === "expanded" && (
                      <>
                        <div className="flex flex-col items-start min-w-0 flex-1 overflow-hidden">
                          <span className="text-sm font-medium truncate leading-tight">
                            {user.username}
                          </span>
                          <Badge
                            variant={ROLE_VARIANT[user.role] ?? "outline"}
                            className="text-[10px] h-4 px-1 leading-none"
                          >
                            {user.role}
                          </Badge>
                        </div>
                        <ChevronUp className="ml-auto h-4 w-4 shrink-0 text-muted-foreground" />
                      </>
                    )}
                  </SidebarMenuButton>
                </DropdownMenuTrigger>

                <DropdownMenuContent side="top" align="start" className="w-48">
                  <DropdownMenuItem asChild>
                    <Link to="/api-keys" className="flex items-center gap-2">
                      <KeyRound className="h-4 w-4" />
                      API Keys
                    </Link>
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={handleLogout}
                    className="flex items-center gap-2 text-destructive focus:text-destructive"
                  >
                    <LogOut className="h-4 w-4" />
                    Sign out
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          )}
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}
