import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { Pressable, View } from "react-native";
import { Notice, Screen, Txt, styles } from "../../components/ui";
import { useStore } from "../../hooks/store";
import { colors } from "../../theme";

function LinkRow({ icon, title, detail, onPress }: { icon: keyof typeof MaterialCommunityIcons.glyphMap; title: string; detail: string; onPress: () => void }) {
  return (
    <Pressable accessibilityRole="button" onPress={onPress} style={({ pressed }) => [styles.dashboardAction, pressed && { backgroundColor: colors.primarySoft }]}>
      <View style={styles.dashboardActionIcon}><MaterialCommunityIcons name={icon} size={21} color={colors.primary} /></View>
      <View style={{ flex: 1, gap: 2 }}><Txt variant="label">{title}</Txt><Txt variant="small" color={colors.muted}>{detail}</Txt></View>
      <MaterialCommunityIcons name="chevron-right" size={23} color={colors.muted} />
    </Pressable>
  );
}

export default function Explore() {
  const s = useStore();
  return (
    <Screen>
      <View style={{ gap: 4 }}><Txt variant="title">Jelajahi latihan</Txt><Txt color={colors.muted}>Gerakan, gym, dan preferensi latihan Anda.</Txt></View>
      {s.catalogError ? <Notice error message={s.catalogError} /> : <Notice message={s.catalogLoading ? "Memuat katalog gerakan dari AmbatuMove…" : `${s.catalog.length} gerakan tersedia dari katalog AmbatuMove.`} />}
      {s.catalogError && s.catalog.length > 0 ? <Notice message="Yang terlihat adalah data tersimpan dari sinkronisasi sukses sebelumnya; perbarui saat jaringan tersedia." /> : null}
      {s.gymsError ? <Notice error message={`Direktori gym belum termuat. ${s.gymsError}`} /> : null}
      {s.gymsError && s.gyms.length > 0 ? <Notice message="Daftar yang terlihat adalah data tersimpan dari sinkronisasi sukses sebelumnya." /> : null}
      <View style={{ gap: 8 }}>
        <LinkRow icon="dumbbell" title="Katalog gerakan" detail={s.catalogLoading ? "Memuat data langsung" : `${s.catalog.length} gerakan`} onPress={() => router.push("/catalog")} />
        <LinkRow icon="map-marker-outline" title="Direktori gym" detail={s.gymsLoading ? "Memuat data langsung" : `${s.gyms.length} lokasi tercatat`} onPress={() => router.push("/gyms")} />
        <LinkRow icon="star-outline" title="Gerakan favorit" detail="Pilihan Anda tersimpan di perangkat" onPress={() => router.push("/favorites")} />
        <LinkRow icon="tune-variant" title="Preferensi tersimpan" detail="Buat rencana berulang lebih cepat" onPress={() => router.push("/presets")} />
      </View>
    </Screen>
  );
}
