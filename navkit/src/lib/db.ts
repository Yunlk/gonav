import { openDB, type DBSchema, type IDBPDatabase } from "idb";
import type { Profile } from "../types";

export interface StoredImage {
  id: string;
  name: string;
  blob: Blob;
}

interface PluginRecord {
  pluginId: string;
  key: string;
  value: unknown;
}

interface NavkitDB extends DBSchema {
  profile: { key: string; value: Profile };
  images: { key: string; value: StoredImage };
  pluginData: { key: [string, string]; value: PluginRecord };
}

const DB_NAME = "navkit";
const DB_VERSION = 2;
const PROFILE_KEY = "current";

let dbPromise: Promise<IDBPDatabase<NavkitDB>> | null = null;

function db(): Promise<IDBPDatabase<NavkitDB>> {
  if (!dbPromise) {
    dbPromise = openDB<NavkitDB>(DB_NAME, DB_VERSION, {
      upgrade(database, oldVersion) {
        if (!database.objectStoreNames.contains("profile")) database.createObjectStore("profile");
        if (!database.objectStoreNames.contains("images")) database.createObjectStore("images", { keyPath: "id" });
        if (oldVersion < 2 && !database.objectStoreNames.contains("pluginData")) {
          database.createObjectStore("pluginData", { keyPath: ["pluginId", "key"] });
        }
      },
    });
  }
  return dbPromise;
}

export async function loadProfile(): Promise<Profile | null> {
  try {
    const value = await (await db()).get("profile", PROFILE_KEY);
    return value ?? null;
  } catch {
    return null;
  }
}

export async function saveProfile(profile: Profile): Promise<void> {
  await (await db()).put("profile", profile, PROFILE_KEY);
}

export async function putImage(image: StoredImage): Promise<void> {
  await (await db()).put("images", image);
}

export async function getImage(id: string): Promise<StoredImage | null> {
  try {
    return (await (await db()).get("images", id)) ?? null;
  } catch {
    return null;
  }
}

export async function listImages(): Promise<StoredImage[]> {
  try {
    return await (await db()).getAll("images");
  } catch {
    return [];
  }
}

export async function deleteImage(id: string): Promise<void> {
  await (await db()).delete("images", id);
}

export async function clearEverything(): Promise<void> {
  const database = await db();
  await database.clear("profile");
  await database.clear("images");
  await database.clear("pluginData");
}

/** 插件存储：按 pluginId 分段，插件之间互不可见。 */
export async function openPluginStore() {
  const database = await db();
  return {
    async get(pluginId: string, key: string): Promise<unknown> {
      const record = await database.get("pluginData", [pluginId, key]);
      return record?.value ?? null;
    },
    async put(value: unknown, pluginId: string, key: string): Promise<void> {
      await database.put("pluginData", { pluginId, key, value });
    },
    async delete(pluginId: string, key: string): Promise<void> {
      await database.delete("pluginData", [pluginId, key]);
    },
    async clear(pluginId: string): Promise<void> {
      const range = IDBKeyRange.bound([pluginId, ""], [pluginId, "\uffff"]);
      await database.delete("pluginData", range);
    },
  };
}
