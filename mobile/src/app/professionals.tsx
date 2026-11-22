import { useEffect, useState } from "react";
import { View } from "react-native";
import { Button, Notice, Screen, Txt, styles } from "../components/ui";
import {
  describeApiError,
  Professional,
  getProfessionals,
} from "../services/ambatumove";
import { colors } from "../theme";

export default function Professionals() {
  const [people, setPeople] = useState<Professional[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const load = () => {
    setLoading(true);
    setError("");
    void getProfessionals()
      .then(setPeople)
      .catch((e) => setError(describeApiError(e)))
      .finally(() => setLoading(false));
  };
  useEffect(() => {
    let live = true;
    void getProfessionals()
      .then((result) => {
        if (live) setPeople(result);
      })
      .catch((e) => {
        if (live) setError(describeApiError(e));
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, []);
  return (
    <Screen>
      <Txt variant="title">Bantuan medis profesional</Txt>
      <Txt color={colors.muted}>
        Direktori publik AmbatuFit untuk membantu Anda menemukan tenaga
        kesehatan terkait aktivitas fisik dan gizi.
      </Txt>
      <Notice message="Informasi ini bukan diagnosis atau layanan darurat. AmbatuMove tidak mengirim data kesehatan atau booking konsultasi." />
      {loading && <Notice message="Memuat direktori AmbatuFit…" />}
      {!!error && (
        <Notice error message={`Direktori belum dapat dimuat. ${error}`} />
      )}
      {!!error && <Button secondary title="Coba lagi" onPress={load} />}
      {!loading && !error && people.length === 0 && (
        <Notice message="Belum ada tenaga profesional aktif di direktori." />
      )}
      {people.map((person) => (
        <View key={person.id} style={styles.card}>
          <Txt variant="exercise">{person.displayName}</Txt>
          <Txt variant="label">
            {person.specialty === "doctor"
              ? "Dokter"
              : person.specialty === "dietitian"
                ? "Ahli gizi"
                : person.specialty}
          </Txt>
          {person.description ? <Txt>{person.description}</Txt> : null}
          <Txt variant="small" color={colors.muted}>
            Profil dari direktori publik AmbatuFit · ID {person.id}
          </Txt>
        </View>
      ))}
      {!!people.length && (
        <Notice message="Direktori publik tidak menyediakan kontak atau pemesanan. Verifikasi kualifikasi dan layanan melalui kanal resmi penyedia sebelum membuat janji." />
      )}
    </Screen>
  );
}
