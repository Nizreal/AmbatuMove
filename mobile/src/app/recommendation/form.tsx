import { useEffect, useState } from "react";
import { View } from "react-native";
import { router } from "expo-router";
import {
  Button,
  Chips,
  Notice,
  Screen,
  SectionTitle,
  Txt,
  styles,
} from "../../components/ui";
import { useStore } from "../../hooks/store";
import { labels, validateConstraints } from "../../domain/workout";
import {
  ApiError,
  EquipmentOption,
  describeApiError,
  generateWorkout,
  getGymEquipment,
  saveFitnessPreferences,
} from "../../services/ambatumove";
import { colors, space } from "../../theme";

export default function Form() {
  const s = useStore();
  const c = s.conditions;
  const { gyms, gymsLoading, gymsError, refreshGyms } = s;
  const { setConditions } = s;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [inventory, setInventory] = useState<{
    gymId: string;
    items: EquipmentOption[];
    error: string;
  }>({ gymId: "", items: [], error: "" });
  const set = (patch: Partial<typeof c>) => s.setConditions({ ...c, ...patch });

  useEffect(() => {
    let live = true;
    if (c.environment !== "gym" || !c.gymId)
      return () => {
        live = false;
      };
    void getGymEquipment(c.gymId)
      .then((items) => {
        if (!live) return;
        setInventory({ gymId: c.gymId!, items, error: "" });
        setConditions((previous) =>
          previous.equipmentSource === "gym_inventory"
            ? { ...previous, equipment: items.map((item) => item.id) }
            : previous,
        );
      })
      .catch((e) => {
        if (live)
          setInventory({
            gymId: c.gymId!,
            items: [],
            error: describeApiError(e),
          });
      });
    return () => {
      live = false;
    };
  }, [c.environment, c.gymId, setConditions]);

  useEffect(() => {
    if (c.environment === "gym" && !gyms.length && !gymsLoading && !gymsError)
      void refreshGyms();
  }, [c.environment, gyms.length, gymsLoading, gymsError, refreshGyms]);

  async function generate() {
    const invalid = validateConstraints(c);
    if (invalid) return setError(invalid);
    if (!s.catalog.length) {
      setError(
        s.catalogError || "Katalog AmbatuMove belum tersedia. Coba muat ulang.",
      );
      return;
    }
    setBusy(true);
    setError("");
    try {
      s.setPlan(await s.withAuth(async (token) => {
        let preferenceWarning = "";
        try { await saveFitnessPreferences(token, c); }
        catch (preferenceError) {
          if (preferenceError instanceof ApiError && preferenceError.kind === "authentication") throw preferenceError;
          preferenceWarning = "Preferensi akun belum tersinkron. Rencana dibuat dari pilihan formulir ini.";
        }
        const generated = await generateWorkout(c, s.catalog, token);
        return preferenceWarning ? { ...generated, warnings: [...(generated.warnings ?? []), preferenceWarning] } : generated;
      }));
      router.push("/recommendation/result");
    } catch (e) {
      if (e instanceof ApiError && e.code === "UNFEASIBLE_PLAN") {
        setError(
          "Belum ada rencana yang memenuhi semua pilihan latihan saat ini. Pilihan Anda tetap tersimpan. Coba ubah durasi, ruang gerak, fokus, atau alat yang tersedia.",
        );
      } else {
        setError(describeApiError(e));
      }
    } finally {
      setBusy(false);
    }
  }

  const availableEquipment = s.equipment.map((item) => item.id);
  const inventoryItems = inventory.gymId === c.gymId ? inventory.items : [];
  const selectedInventory = inventoryItems.map((item) => item.id);
  const inventoryLoading =
    c.environment === "gym" && !!c.gymId && inventory.gymId !== c.gymId;
  const inventoryError = inventory.gymId === c.gymId ? inventory.error : "";
  const equipmentChoices =
    c.environment === "gym" &&
    c.equipmentSource === "gym_inventory" &&
    selectedInventory.length
      ? selectedInventory
      : availableEquipment;
  const equipmentLabels = Object.fromEntries(
    s.equipment.map((item) => [item.id, item.label]),
  );
  const focusNames = {
    full: "Seluruh tubuh",
    upper: "Tubuh atas",
    lower: "Tubuh bawah",
    core: "Inti tubuh",
    chest: "Dada",
    back: "Punggung",
    biceps: "Biseps",
    triceps: "Triseps",
    shoulders: "Bahu",
    quadriceps: "Paha depan",
    hamstrings: "Paha belakang",
    glutes: "Gluteus",
    calves: "Betis",
    forearms: "Lengan bawah",
  };

  return (
    <Screen
      footer={
        <Button
          title="Dapatkan rekomendasi"
          busy={busy}
          onPress={() => void generate()}
        />
      }
    >
      <View style={{ gap: space.xs }}>
        <Txt variant="title">Rekomendasi workout</Txt>
        <Txt color={colors.muted}>
          Rencana dibuat AmbatuMove berdasarkan katalog dan alat yang tersedia.
        </Txt>
      </View>
      {!!error && <Notice error message={error} />}
      {s.catalogLoading && (
        <Notice message="Memuat katalog gerakan AmbatuMove…" />
      )}
      {s.catalogError && <Notice error message={s.catalogError} />}
      <View style={styles.formSection}>
        <SectionTitle>Tujuan latihan</SectionTitle>
        <Chips
          values={["general", "strength", "cardio"] as const}
          selected={c.goal}
          names={labels}
          onSelect={(goal) => set({ goal })}
        />
      </View>
      <View style={styles.formSection}>
        <SectionTitle>Fokus latihan</SectionTitle>
        <Chips
          values={Object.keys(focusNames)}
          selected={c.focus}
          names={focusNames}
          onSelect={(focus) => set({ focus })}
        />
      </View>
      <View style={styles.formSection}>
        <SectionTitle>Pengalaman</SectionTitle>
        <Chips
          values={["beginner", "intermediate", "advanced"] as const}
          selected={c.experience}
          names={labels}
          onSelect={(experience) => set({ experience })}
        />
      </View>
      <View style={styles.formSection}>
        <SectionTitle>Waktu tersedia</SectionTitle>
        <Chips
          values={["10", "15", "20", "30", "45", "60"]}
          selected={String(c.durationMinutes)}
          names={Object.fromEntries(
            [10, 15, 20, 30, 45, 60].map((n) => [n, `${n} menit`]),
          )}
          onSelect={(n) => set({ durationMinutes: Number(n) })}
        />
      </View>
      <View style={styles.formSection}>
        <SectionTitle>Lokasi latihan</SectionTitle>
        <Chips
          values={["home", "gym", "indoor", "outdoor"] as const}
          selected={c.environment}
          names={{
            home: "Rumah",
            gym: "Gym",
            indoor: "Dalam ruangan",
            outdoor: "Luar ruangan",
          }}
          onSelect={(environment) =>
            set({
              environment,
              gymId: undefined,
              equipment: environment === "gym" ? [] : ["bodyweight"],
              equipmentSource: "explicit",
            })
          }
        />
      </View>
      {c.environment === "gym" && (
        <View style={styles.formSection}>
          <SectionTitle>Pilih gym</SectionTitle>
          {s.gymsLoading && <Notice message="Memuat direktori gym…" />}
          {s.gymsError && <Notice error message={s.gymsError} />}
          {!s.gymsLoading && !s.gyms.length && (
            <Button
              secondary
              title="Muat daftar gym"
              onPress={() => void s.refreshGyms()}
            />
          )}
          {!!s.gyms.length && (
            <Chips
              values={s.gyms.map((gym) => gym.id)}
              selected={c.gymId ?? ""}
              names={Object.fromEntries(
                s.gyms.map((gym) => [gym.id, gym.name]),
              )}
              onSelect={(gymId) =>
                set({ gymId, equipment: [], equipmentSource: "gym_inventory" })
              }
            />
          )}
          {!!c.gymId && (
            <Txt variant="small" color={colors.muted}>
              Status data:{" "}
              {s.gyms.find((gym) => gym.id === c.gymId)?.verificationStatus ??
                "belum diverifikasi"}
            </Txt>
          )}
          {!!c.gymId && (
            <>
              {inventoryLoading && <Notice message="Memuat inventaris gym…" />}
              {inventoryError && (
                <Notice
                  error
                  message={`Inventaris gym belum bisa dimuat. ${inventoryError} Pilih alat secara manual.`}
                />
              )}
              {!inventoryLoading &&
                !inventoryError &&
                selectedInventory.length === 0 && (
                  <Notice message="Gym ini belum memiliki inventaris alat yang tercatat. Pilih alat secara manual agar rekomendasi tetap sesuai." />
                )}
              {selectedInventory.length > 0 && (
                <Chips
                  values={["gym_inventory", "explicit"]}
                  selected={c.equipmentSource ?? "gym_inventory"}
                  names={{
                    gym_inventory: "Pakai inventaris gym",
                    explicit: "Pilih alat sendiri",
                  }}
                  onSelect={(equipmentSource) =>
                    set({
                      equipmentSource: equipmentSource as
                        "gym_inventory" | "explicit",
                      equipment:
                        equipmentSource === "gym_inventory"
                          ? selectedInventory
                          : [],
                    })
                  }
                />
              )}
            </>
          )}
        </View>
      )}
      <View style={styles.formSection}>
        <SectionTitle>Ruang gerak</SectionTitle>
        <Chips
          values={["small", "medium", "large", "unrestricted"]}
          selected={c.space}
          names={{
            small: "Terbatas",
            medium: "Sedang",
            large: "Luas",
            unrestricted: "Bebas",
          }}
          onSelect={(spaceValue) => set({ space: spaceValue })}
        />
      </View>
      <View style={styles.formSection}>
        <SectionTitle>Alat tersedia</SectionTitle>
        {c.environment === "gym" &&
        c.equipmentSource === "gym_inventory" &&
        selectedInventory.length > 0 ? (
          <Txt variant="small" color={colors.muted}>
            Rekomendasi menggunakan inventaris gym yang terdaftar.
          </Txt>
        ) : c.environment === "gym" &&
          c.equipmentSource === "gym_inventory" &&
          inventoryLoading ? (
          <Notice message="Tunggu sampai inventaris gym selesai dimuat." />
        ) : equipmentChoices.length ? (
          <Chips
            values={equipmentChoices}
            selected={c.equipment}
            names={equipmentLabels}
            onSelect={(id) =>
              set({
                equipmentSource: "explicit",
                equipment: c.equipment.includes(id)
                  ? c.equipment.filter((value) => value !== id)
                  : [
                      ...c.equipment.filter((value) => value !== "bodyweight"),
                      id,
                    ],
              })
            }
          />
        ) : (
          <Notice message="Daftar alat AmbatuMove sedang dimuat." />
        )}
      </View>
      <View style={styles.formSection}>
        <SectionTitle>Intensitas latihan</SectionTitle>
        <Chips
          values={["low", "moderate", "vigorous"] as const}
          names={{ low: "Ringan", moderate: "Sedang", vigorous: "Tinggi" }}
          selected={c.intensity}
          onSelect={(intensity) => set({ intensity })}
        />
        <Txt variant="small" color={colors.muted}>
          Intensitas diteruskan ke perencana AmbatuMove.
        </Txt>
      </View>
      {c.environment === "gym" && !!c.gymId && (
        <Notice message="Direktori gym berisi data fasilitas yang bisa belum terverifikasi. Periksa alat di lokasi sebelum latihan." />
      )}
    </Screen>
  );
}
