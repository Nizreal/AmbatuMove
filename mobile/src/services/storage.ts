import AsyncStorage from "@react-native-async-storage/async-storage";

export const keys = {
  history: "@ambatumove/history/v1",
  draft: "@ambatumove/draft/v1",
  active: "@ambatumove/active/v1",
  catalog: "@ambatumove/catalog-cache/v1",
  equipment: "@ambatumove/equipment-cache/v1",
  gyms: "@ambatumove/gyms-cache/v1",
};

export async function read<T>(key: string, fallback: T): Promise<T> {
  const value = await AsyncStorage.getItem(key);
  return value ? (JSON.parse(value) as T) : fallback;
}

export const write = (key: string, value: unknown) =>
  AsyncStorage.setItem(key, JSON.stringify(value));
