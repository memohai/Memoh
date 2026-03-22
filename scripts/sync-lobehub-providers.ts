#!/usr/bin/env bun

import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";

interface ExistingProviderFile {
  filePath: string;
  headerLines: string[];
  providerId: string;
}

interface ProviderModelOutput {
  config: {
    compatibilities?: string[];
    context_window?: number;
    dimensions?: number;
  };
  model_id: string;
  name: string;
  type: string;
}

interface UpstreamModel {
  abilities?: Record<string, boolean | undefined>;
  contextWindowTokens?: number;
  displayName?: string;
  id: string;
  maxDimension?: number;
  type?: string;
}

const AI_MODELS_DIR = "packages/model-bank/src/aiModels";
const PROVIDERS_DIR = "conf/providers";

const projectRoot = path.resolve(import.meta.dir, "..");

const COMPATIBILITY_ORDER = [
  "vision",
  "tool-call",
  "reasoning",
  "image-output",
] as const;

const COMPATIBILITY_MAP = {
  functionCall: "tool-call",
  imageOutput: "image-output",
  reasoning: "reasoning",
  vision: "vision",
} as const;

const SUPPORTED_MODEL_TYPES = new Set(["chat", "embedding"]);

const resolveLobeHubRoot = () => {
  const candidates = [
    process.env.LOBEHUB_PATH,
    path.resolve(projectRoot, "../lobehub"),
    path.resolve(projectRoot, "../lobe-chat"),
  ].filter(Boolean) as string[];

  if (candidates.length === 0) {
    throw new Error(
      "Could not resolve LobeHub path. Set LOBEHUB_PATH or place lobehub next to Memoh.",
    );
  }

  return candidates[0]!;
};

const importFromFile = async <T>(filePath: string): Promise<T> => {
  const moduleUrl = pathToFileURL(filePath).href;
  return import(moduleUrl) as Promise<T>;
};

const parseExistingProviderFile = async (
  filePath: string,
): Promise<ExistingProviderFile> => {
  const raw = await readFile(filePath, "utf8");
  const lines = raw.replaceAll("\r\n", "\n").trimEnd().split("\n");
  const modelsIndex = lines.findIndex((line) => line.trim() === "models:");

  if (modelsIndex === -1) {
    throw new Error(`Provider file is missing models section: ${filePath}`);
  }

  return {
    filePath,
    headerLines: lines.slice(0, modelsIndex),
    providerId: path.basename(filePath, path.extname(filePath)),
  };
};

const toCompatibilities = (abilities?: UpstreamModel["abilities"]) => {
  if (!abilities) return undefined;

  const compatibilities = Object.entries(abilities)
    .filter(([, enabled]) => Boolean(enabled))
    .map(([key]) => COMPATIBILITY_MAP[key as keyof typeof COMPATIBILITY_MAP])
    .filter((value): value is (typeof COMPATIBILITY_ORDER)[number] =>
      Boolean(value),
    )
    .sort(
      (left, right) =>
        COMPATIBILITY_ORDER.indexOf(left) - COMPATIBILITY_ORDER.indexOf(right),
    );

  return compatibilities.length > 0 ? compatibilities : undefined;
};

const toOutputModel = (model: UpstreamModel): ProviderModelOutput => {
  if (model.type === "embedding") {
    return {
      config: {
        ...(typeof model.maxDimension === "number"
          ? { dimensions: model.maxDimension }
          : {}),
      },
      model_id: model.id,
      name: model.displayName || model.id,
      type: "embedding",
    };
  }

  const compatibilities = toCompatibilities(model.abilities);

  return {
    config: {
      ...(compatibilities ? { compatibilities } : {}),
      ...(typeof model.contextWindowTokens === "number"
        ? { context_window: model.contextWindowTokens }
        : {}),
    },
    model_id: model.id,
    name: model.displayName || model.id,
    type: model.type || "chat",
  };
};

const isUpstreamModel = (value: unknown): value is UpstreamModel => {
  if (!value || typeof value !== "object") return false;

  return typeof (value as { id?: unknown }).id === "string";
};

const loadProviderModels = async (lobeHubRoot: string, providerId: string) => {
  const filePath = path.join(lobeHubRoot, AI_MODELS_DIR, `${providerId}.ts`);
  const module = await importFromFile<{ default?: unknown }>(filePath);

  if (!Array.isArray(module.default)) {
    throw new Error(
      `Upstream provider module does not export a default model array: ${filePath}`,
    );
  }

  const dedupedModels = new Map<string, UpstreamModel>();

  for (const item of module.default) {
    if (!isUpstreamModel(item)) continue;
    if (!item.type || !SUPPORTED_MODEL_TYPES.has(item.type)) continue;
    if (dedupedModels.has(item.id)) continue;

    dedupedModels.set(item.id, item);
  }

  return [...dedupedModels.values()];
};

const renderCompatibilities = (compatibilities: string[]) =>
  `[${compatibilities.join(", ")}]`;

const renderModel = (model: ProviderModelOutput) => {
  const lines = [
    `  - model_id: ${model.model_id}`,
    `    name: ${model.name}`,
    `    type: ${model.type}`,
    "    config:",
  ];

  if (model.config.compatibilities) {
    lines.push(
      `      compatibilities: ${renderCompatibilities(model.config.compatibilities)}`,
    );
  }

  if (typeof model.config.context_window === "number") {
    lines.push(`      context_window: ${model.config.context_window}`);
  }

  if (typeof model.config.dimensions === "number") {
    lines.push(`      dimensions: ${model.config.dimensions}`);
  }

  return lines.join("\n");
};

const renderProviderYaml = (
  providerFile: ExistingProviderFile,
  models: ProviderModelOutput[],
) => {
  return [
    ...providerFile.headerLines,
    "models:",
    models.map(renderModel).join("\n\n"),
    "",
  ].join("\n");
};

const main = async () => {
  const lobeHubRoot = resolveLobeHubRoot();
  const providerDirectory = path.join(projectRoot, PROVIDERS_DIR);
  const providerFiles = (await readdir(providerDirectory))
    .filter((fileName) => fileName.endsWith(".yaml"))
    .map((fileName) => path.join(providerDirectory, fileName))
    .sort();

  const parsedProviderFiles = await Promise.all(
    providerFiles.map(parseExistingProviderFile),
  );

  for (const providerFile of parsedProviderFiles) {
    const upstreamModels = await loadProviderModels(
      lobeHubRoot,
      providerFile.providerId,
    );
    const syncedModels = upstreamModels.map(toOutputModel);
    const yaml = renderProviderYaml(providerFile, syncedModels);

    await mkdir(path.dirname(providerFile.filePath), { recursive: true });
    await writeFile(providerFile.filePath, yaml, "utf8");

    console.log(
      `synced ${providerFile.providerId}: ${path.relative(projectRoot, providerFile.filePath)} (${syncedModels.length} models)`,
    );
  }
};

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
