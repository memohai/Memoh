export interface Config {
  log: LogConfig;
  server: ServerConfig;
  admin: AdminConfig;
  auth: AuthConfig;
  containerd: ContainerdConfig;
  mcp: McpConfig;
  postgres: PostgresConfig;
  qdrant: QdrantConfig;
  agent_gateway: AgentGatewayConfig;
  web: WebConfig;
}

export interface LogConfig {
  level: string;
  format: string;
}

export interface ServerConfig {
  addr: string;
}

export interface AdminConfig {
  username: string;
  password: string;
  email: string;
}

export interface AuthConfig {
  jwt_secret: string;
  jwt_expires_in: string;
}

export interface ContainerdConfig {
  socket_path: string;
  namespace: string;
}

export interface McpConfig {
  image: string;
  snapshotter: string;
  data_root: string;
}

export interface PostgresConfig {
  host: string;
  port: number;
  user: string;
  password: string;
  database: string;
  sslmode: string;
}

export interface QdrantConfig {
  base_url: string;
  api_key: string;
  collection: string;
  timeout_seconds: number;
}

export interface AgentGatewayConfig {
  host: string;
  port: number;
  server_addr?: string;
}

export interface WebConfig {
  host: string;
  port: number;
}

