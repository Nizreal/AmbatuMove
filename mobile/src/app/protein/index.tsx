import AsyncStorage from "@react-native-async-storage/async-storage";
import { useLocalSearchParams, router } from "expo-router";
import { useEffect, useMemo, useState } from "react";
import { Alert, Pressable, View } from "react-native";
import {
  Button,
  Input,
  Notice,
  Progress,
  Screen,
  Txt,
  styles,
} from "../../components/ui";
import { useStore } from "../../hooks/store";
import { colors, space } from "../../theme";

type ProteinEntry = {
  id: string;
  name: string;
  proteinGrams: number;
  barcode?: string;
  createdAt: string;
};
type ProteinData = { goalGrams: number; entries: ProteinEntry[] };
const localDay = (date: Date) => {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
};
const today = () => {
  const d = new Date();
  return localDay(d);
};
const keyFor = (account: string) =>
  `@ambatunourish/protein/v1/${encodeURIComponent(account.toLowerCase())}`;

export default function ProteinTracker() {
  const s = useStore();
  const params = useLocalSearchParams<{ barcode?: string }>();
  const [data, setData] = useState<ProteinData>({
    goalGrams: 100,
    entries: [],
  });
  const [loadedFor, setLoadedFor] = useState("");
  const [name, setName] = useState("");
  const [grams, setGrams] = useState("");
  const [error, setError] = useState("");
  const [goalInput, setGoalInput] = useState("100");
  const storageKey = keyFor(s.user ?? "demo@ambatumove.app");
  const ready = loadedFor === storageKey;

  useEffect(() => {
    let alive = true;
    void AsyncStorage.getItem(storageKey)
      .then((raw) => {
        if (!alive) return;
        const parsed = raw
          ? (JSON.parse(raw) as ProteinData)
          : { goalGrams: 100, entries: [] };
        setData(parsed);
        setGoalInput(String(parsed.goalGrams));
        setLoadedFor(storageKey);
      })
      .catch(() => {
        if (alive) {
          setData({ goalGrams: 100, entries: [] });
          setGoalInput("100");
          setError(
            "Data protein lokal belum dapat dibaca. Data sebelumnya tetap tersimpan.",
          );
          setLoadedFor(storageKey);
        }
      });
    return () => {
      alive = false;
    };
  }, [storageKey]);

  const capturedBarcode =
    typeof params.barcode === "string" ? params.barcode : "";
  const todaysEntries = useMemo(
    () =>
      ready
        ? data.entries.filter(
            (entry) => localDay(new Date(entry.createdAt)) === today(),
          )
        : [],
    [data.entries, ready],
  );
  const totalGrams = todaysEntries.reduce(
    (total, entry) => total + entry.proteinGrams,
    0,
  );
  const update = async (next: ProteinData) => {
    setData(next);
    try {
      await AsyncStorage.setItem(storageKey, JSON.stringify(next));
      setError("");
    } catch {
      setError(
        "Perubahan belum tersimpan di perangkat. Coba lagi sebelum menutup aplikasi.",
      );
    }
  };
  const addEntry = () => {
    const value = Number(grams);
    if (!name.trim() || !Number.isFinite(value) || value <= 0 || value > 250) {
      setError("Isi nama makanan dan protein per sajian antara 0,1–250 gram.");
      return;
    }
    void update({
      ...data,
      entries: [
        {
          id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
          name: name.trim(),
          proteinGrams: value,
          barcode: capturedBarcode || undefined,
          createdAt: new Date().toISOString(),
        },
        ...data.entries,
      ],
    });
    setName("");
    setGrams("");
    if (typeof params.barcode === "string")
      router.setParams({ barcode: undefined });
  };
  const saveGoal = () => {
    const value = Number(goalInput);
    if (!Number.isFinite(value) || value < 1 || value > 400)
      return setError("Target protein harus 1–400 gram per hari.");
    void update({ ...data, goalGrams: Math.round(value) });
  };

  return (
    <Screen>
      <View style={{ gap: space.xs }}>
        <Txt variant="title">Protein harian</Txt>
        <Txt color={colors.muted}>
          Catat asupan protein secara sederhana. Data disimpan lokal per akun.
        </Txt>
      </View>
      {!!error && <Notice error message={error} />}
      <View style={styles.planCard}>
        <Txt variant="label">Target hari ini</Txt>
        <View
          style={{ flexDirection: "row", alignItems: "center", gap: space.sm }}
        >
          <Input
            label="Gram per hari"
            keyboardType="number-pad"
            value={goalInput}
            onChangeText={setGoalInput}
          />
          <Button
            title="Simpan"
            secondary
            onPress={saveGoal}
            disabled={!ready}
          />
        </View>
        <Txt variant="heading">
          {totalGrams} / {data.goalGrams} g
        </Txt>
        <Progress
          value={totalGrams / Math.max(1, data.goalGrams)}
          label="Protein tercatat hari ini"
        />
      </View>
      <View style={styles.formSection}>
        <Txt variant="label">Tambahkan asupan</Txt>
        <Button
          secondary
          title="Pindai barcode produk"
          onPress={() => router.push("/protein/scan")}
        />
        <Input
          label="Nama makanan"
          value={name}
          onChangeText={setName}
          placeholder="Contoh: susu kedelai"
        />
        <Input
          label="Protein per sajian (gram)"
          value={grams}
          onChangeText={setGrams}
          keyboardType="decimal-pad"
          placeholder="Masukkan sesuai label"
        />
        {capturedBarcode ? (
          <Notice
            message={`Barcode ${capturedBarcode} dipindai. Masukkan jumlah protein dari label kemasan.`}
          />
        ) : null}
        <Button title="Catat asupan" onPress={addEntry} disabled={!ready} />
        <Notice message="Lookup produk AmbatuNourish belum tersedia. Nilai protein tidak diisi otomatis; gunakan angka pada label kemasan." />
      </View>
      <View style={styles.formSection}>
        <Txt variant="label">Asupan hari ini</Txt>
        {!todaysEntries.length ? (
          <Notice message="Belum ada asupan yang dicatat hari ini." />
        ) : (
          todaysEntries.map((entry) => (
            <Pressable
              key={entry.id}
              onLongPress={() =>
                Alert.alert("Hapus catatan protein?", entry.name, [
                  { text: "Batal", style: "cancel" },
                  {
                    text: "Hapus",
                    style: "destructive",
                    onPress: () =>
                      void update({
                        ...data,
                        entries: data.entries.filter(
                          (item) => item.id !== entry.id,
                        ),
                      }),
                  },
                ])
              }
              style={styles.card}
              accessibilityHint="Tekan lama untuk menghapus catatan"
            >
              <View
                style={{
                  flexDirection: "row",
                  justifyContent: "space-between",
                  gap: space.md,
                }}
              >
                <View style={{ flex: 1 }}>
                  <Txt variant="label">{entry.name}</Txt>
                  {entry.barcode ? (
                    <Txt variant="small" color={colors.muted}>
                      Kode {entry.barcode}
                    </Txt>
                  ) : null}
                </View>
                <Txt variant="exercise">{entry.proteinGrams} g</Txt>
              </View>
            </Pressable>
          ))
        )}
        <Txt variant="small" color={colors.muted}>
          Tekan lama catatan untuk menghapusnya.
        </Txt>
      </View>
      {!ready && <Notice message="Memuat catatan lokal…" />}
    </Screen>
  );
}
