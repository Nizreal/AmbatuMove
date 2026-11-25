import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { View } from "react-native";
import { Button, Notice, Screen, Txt, styles } from "../components/ui";
import { colors } from "../theme";

export default function Professionals() {
  return (
    <Screen>
      <Txt variant="title">Bantuan profesional</Txt>
      <View style={styles.emptyState}>
        <MaterialCommunityIcons name="medical-bag" size={32} color={colors.primary} />
        <Txt variant="heading">Direktori AmbatuFit belum tersedia</Txt>
        <Txt color={colors.muted}>Layanan AmbatuFit tidak merespons saat integrasi diperiksa. Kontrak partner yang terdokumentasi memerlukan autentikasi server-side yang tidak tersedia melalui AmbatuMove mobile saat ini.</Txt>
      </View>
      <Notice error message="Tidak ada profil profesional, jadwal, atau kontak yang dimuat. AmbatuMove tidak akan membuat daftar pengganti." />
      <Notice message="Jangan gunakan aplikasi ini untuk diagnosis. Jika Anda mengalami keadaan darurat, hubungi layanan darurat setempat." />
      <Button secondary title="Kembali ke kesehatan" onPress={() => router.replace("/wellness")} />
    </Screen>
  );
}
