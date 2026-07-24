import { GLTFLoader, type GLTF } from 'three/examples/jsm/loaders/GLTFLoader.js';

export type GltfAssetCatalogEntry = {
  source: string;
  resources: Record<string, string>;
};

export const resolveGlobAssetUrl = (
  assetUrls: Record<string, string>,
  rootPrefix: string,
  relativePath: string,
  missingLabel: string,
): string => {
  const normalizedPath = relativePath.replace(/\\/g, '/');
  const key = `${rootPrefix}/${normalizedPath}`;
  const assetUrl = assetUrls[key];
  if (!assetUrl) {
    throw new Error(`Missing ${missingLabel}: ${normalizedPath}`);
  }
  return assetUrl;
};

const collectSiblingResources = (
  assetUrls: Record<string, string>,
  rootPrefix: string,
  relativePath: string,
): Record<string, string> => {
  const normalizedPath = relativePath.replace(/\\/g, '/');
  const lastSlashIndex = normalizedPath.lastIndexOf('/');
  const directory = lastSlashIndex >= 0 ? normalizedPath.slice(0, lastSlashIndex) : '';
  const directoryKeyPrefix = `${rootPrefix}/${directory ? `${directory}/` : ''}`;
  const resources: Record<string, string> = {};
  for (const [assetKey, assetUrl] of Object.entries(assetUrls)) {
    if (!assetKey.startsWith(directoryKeyPrefix) || assetKey.endsWith('.gltf')) {
      continue;
    }
    const relativeName = assetKey.slice(directoryKeyPrefix.length);
    if (relativeName.includes('/')) {
      continue;
    }
    resources[relativeName] = assetUrl;
  }
  return resources;
};

export const registerCatalogedGltfAsset = (
  assetUrls: Record<string, string>,
  rawGltfAssets: Record<string, string>,
  rootPrefix: string,
  relativePath: string,
  missingLabel: string,
  catalog: Map<string, GltfAssetCatalogEntry>,
): string => {
  const normalizedPath = relativePath.replace(/\\/g, '/');
  const assetUrl = resolveGlobAssetUrl(assetUrls, rootPrefix, normalizedPath, missingLabel);
  const rawSource = rawGltfAssets[`${rootPrefix}/${normalizedPath}`];
  if (rawSource) {
    catalog.set(assetUrl, {
      source: rawSource,
      resources: collectSiblingResources(assetUrls, rootPrefix, normalizedPath),
    });
  }
  return assetUrl;
};

const resolveCatalogResourceUrl = (uri: string, resources: Record<string, string>): string | null => {
  const basename = uri.split('/').pop() ?? uri;
  const withoutMimeSuffix = (value: string): string | null => {
    const normalized = value.replace(/\\/g, '/');
    return normalized.replace(/_([a-z0-9]+)\.([a-z0-9]+)$/i, (_match, embeddedExtension: string, actualExtension: string) => {
      if (embeddedExtension.toLowerCase() !== actualExtension.toLowerCase()) {
        return _match;
      }
      return `.${actualExtension}`;
    });
  };
  const candidates = [uri, basename];
  const normalizedUri = withoutMimeSuffix(uri);
  const normalizedBasename = withoutMimeSuffix(basename);
  if (normalizedUri && normalizedUri !== uri) {
    candidates.push(normalizedUri);
  }
  if (normalizedBasename && normalizedBasename !== basename) {
    candidates.push(normalizedBasename);
  }
  candidates.push(...candidates.map((candidate) => candidate.toLowerCase()));
  for (const candidate of candidates) {
    if (resources[candidate]) {
      return resources[candidate];
    }
  }
  for (const [resourceName, resourceUrl] of Object.entries(resources)) {
    if (resourceName.toLowerCase() === uri.toLowerCase() || resourceName.toLowerCase() === basename.toLowerCase()) {
      return resourceUrl;
    }
  }
  return null;
};

export const rewriteGltfExternalResourceUris = (
  source: string,
  resources: Record<string, string>,
  options?: { baseUrl?: string },
): string => {
  const document = JSON.parse(source) as {
    images?: Array<{ uri?: string }>;
    buffers?: Array<{ uri?: string }>;
  };
  const rewriteUri = (uri: string | undefined): void => {
    if (!uri || uri.startsWith('data:')) {
      return;
    }
    const resolvedUrl = resolveCatalogResourceUrl(uri, resources);
    if (!resolvedUrl) {
      throw new Error(`Missing external GLTF resource for "${uri}".`);
    }
    const rewrittenUrl = options?.baseUrl ? new URL(resolvedUrl, options.baseUrl).toString() : resolvedUrl;
    for (const list of [document.images, document.buffers]) {
      list?.forEach((entry) => {
        if (entry.uri === uri) {
          entry.uri = rewrittenUrl;
        }
      });
    }
  };
  document.images?.forEach((entry) => rewriteUri(entry.uri));
  document.buffers?.forEach((entry) => rewriteUri(entry.uri));
  return JSON.stringify(document);
};

export const loadCatalogedGltf = async (
  loader: GLTFLoader,
  url: string,
  getCatalogEntry: (url: string) => GltfAssetCatalogEntry | null,
): Promise<GLTF> => {
  const catalogEntry = getCatalogEntry(url);
  if (!catalogEntry) {
    return loader.loadAsync(url);
  }
  const runtimeBaseUrl = globalThis.location?.origin ? `${globalThis.location.origin}/` : undefined;
  const rewrittenSource = rewriteGltfExternalResourceUris(catalogEntry.source, catalogEntry.resources, {
    baseUrl: runtimeBaseUrl,
  });
  return await new Promise<GLTF>((resolve, reject) => {
    loader.parse(rewrittenSource, '', resolve, reject);
  });
};
