import { useMemo, useState } from "react";
import { FlatList, Pressable, View } from "react-native";
import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router, useLocalSearchParams } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import {
  Button,
  Chips,
  Input,
  Notice,
  Screen,
  Txt,
  styles,
} from "../components/ui";
import { ExerciseInfo, ExercisePhoto } from "../components/exercise";
import { alternatives, Exercise, itemFor, labels } from "../domain/workout";
import { useStore } from "../hooks/store";
import { colors, space } from "../theme";
export default function Catalog() {
  const s = useStore(),
    { swap } = useLocalSearchParams<{ swap?: string }>(),
    [query, setQuery] = useState(""),
    [muscle, setMuscle] = useState("full"),
    [equipment, setEquipment] = useState("all"),
    [selectedId, setSelectedId] = useState("");
  const insets = useSafeAreaInsets();
  const catalog = s.catalog;
  const index = swap === undefined ? null : Number(swap);
  const candidates = useMemo(
    () =>
      index !== null && s.plan && s.plan.items[index]
        ? alternatives(catalog, s.plan, index)
        : catalog,
    [index, s.plan, catalog],
  );
  const results = candidates.filter(
    (e) =>
      (e.name + " " + labels[e.muscle])
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (muscle === "full" || e.muscle === muscle) &&
      (equipment === "all" ||
        (equipment === "none"
          ? e.equipment.length === 0
          : e.equipment.includes(equipment))),
  );
  function choose(e: Exercise) {
    const p = s.plan;
    if (!p) return;
    const item = itemFor(e, p.constraints);
    if (index !== null) {
      const updated = {
        ...p,
        items: p.items.map((old, i) =>
          i === index
            ? {
                ...item,
                sets: old.sets,
                rest: old.rest,
                target:
                  old.exercise.timed === e.timed ? old.target : item.target,
              }
            : old,
        ),
      };
      s.setPlan(updated);
    } else {
      s.setPlan({ ...p, items: [...p.items, item] });
    }
    router.back();
  }
  const current = index !== null ? s.plan?.items[index] : null;
  if (index !== null && current) {
    const selected = results.find((e) => e.id === selectedId);
    return (
      <Screen
        footer={
          <Button
            title="Simpan perubahan"
            disabled={!selected}
            onPress={() => selected && choose(selected)}
          />
        }
      >
        <Txt variant="title">Ganti gerakan</Txt>
        <Notice
          message={`Pilih pengganti dengan fokus ${labels[current.exercise.muscle].toLowerCase()} yang sama dan sesuai kondisi latihan.`}
        />
        <View style={styles.planCard}>
          <Txt variant="small" color={colors.primary}>
            Gerakan saat ini
          </Txt>
          <View style={styles.planRow}>
            <ExercisePhoto exercise={current.exercise} compact />
            <View style={{ flex: 1 }}>
              <Txt variant="exercise">{current.exercise.name}</Txt>
              <Txt variant="small">
                Fokus: {labels[current.exercise.muscle]}
              </Txt>
            </View>
          </View>
        </View>
        <Txt variant="label">Pilih gerakan alternatif</Txt>
        {results.map((exercise) => {
          const active = selectedId === exercise.id;
          return (
            <Pressable
              key={exercise.id}
              accessibilityRole="radio"
              accessibilityState={{ selected: active }}
              onPress={() => setSelectedId(exercise.id)}
              style={({ pressed }) => [
                styles.alternativeRow,
                active && styles.alternativeSelected,
                pressed && { opacity: 0.8 },
              ]}
            >
              <ExercisePhoto exercise={exercise} compact />
              <View style={{ flex: 1, gap: 2 }}>
                <Txt variant="exercise">{exercise.name}</Txt>
                <Txt variant="small" color={colors.muted}>
                  Fokus: {labels[exercise.muscle]} · {labels[exercise.level]}
                </Txt>
                <Txt variant="small">
                  {exercise.equipment.length
                    ? exercise.equipment.map((e) => labels[e]).join(", ")
                    : "Tanpa alat"}
                </Txt>
              </View>
              <MaterialCommunityIcons
                name={active ? "radiobox-marked" : "radiobox-blank"}
                size={22}
                color={active ? colors.primary : colors.muted}
              />
            </Pressable>
          );
        })}
        {!results.length && (
          <Notice message="Tidak ada alternatif yang cocok dengan kondisi ini." />
        )}
      </Screen>
    );
  }
  return (
    <FlatList
      style={styles.fill}
      contentContainerStyle={{
        padding: space.xl,
        paddingBottom: insets.bottom + space.xl,
        gap: space.xl,
      }}
      keyboardShouldPersistTaps="handled"
      data={results}
      keyExtractor={(e) => e.id}
      ListHeaderComponent={
        <View style={styles.group}>
          <Txt variant="title">
            {index === null ? "Temukan gerakan." : "Pilihan pengganti."}
          </Txt>
          <Txt>
            {index === null
              ? `${catalog.length} gerakan dari katalog AmbatuMove`
              : "Pilihan dari katalog AmbatuMove dengan fokus dan alat yang sesuai."}
          </Txt>
          {s.catalogLoading && <Notice message="Memuat katalog resmi…" />}
          {s.catalogError && <Notice error message={s.catalogError} />}
          {s.catalogError && !s.catalogLoading && (
            <Button
              secondary
              title="Coba hubungkan ulang"
              onPress={() => void s.refreshCatalog()}
            />
          )}
          <Input
            label="Cari gerakan atau otot"
            value={query}
            onChangeText={setQuery}
          />
          {index === null && (
            <>
              <Chips
                values={[
                  "full",
                  ...Array.from(new Set(catalog.map((e) => e.muscle))),
                ]}
                selected={muscle}
                names={{ ...labels, full: "Semua otot" }}
                onSelect={setMuscle}
              />
              <Chips
                values={["all", ...s.equipment.map((item) => item.id)]}
                selected={equipment}
                names={{
                  ...labels,
                  ...Object.fromEntries(
                    s.equipment.map((item) => [item.id, item.label]),
                  ),
                  all: "Semua alat",
                }}
                onSelect={setEquipment}
              />
            </>
          )}
          <Txt variant="small">{results.length} pilihan ditemukan</Txt>
        </View>
      }
      ListEmptyComponent={
        <Notice message="Tidak ada gerakan yang cocok. Ubah pencarian atau kembali untuk mengubah kondisi." />
      }
      renderItem={({ item }) => {
        const added =
          index === null &&
          s.plan?.items.some((p) => p.exercise.id === item.id);
        return (
          <View style={styles.card}>
            <ExercisePhoto exercise={item} />
            <ExerciseInfo exercise={item} />
            <Button
              secondary
              title="Lihat panduan"
              onPress={() =>
                router.push({
                  pathname: "/exercise/[id]",
                  params: { id: item.id },
                })
              }
            />
            <Button
              title={added ? "Sudah ditambahkan" : "Tambahkan gerakan"}
              disabled={added || !s.plan}
              onPress={() => choose(item)}
            />
          </View>
        );
      }}
    />
  );
}
