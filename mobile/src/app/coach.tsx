import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { useRef, useState } from "react";
import { router } from "expo-router";
import { Pressable, ScrollView, TextInput, View } from "react-native";
import { Button, Notice, Screen, Txt, styles } from "../components/ui";
import { useStore } from "../hooks/store";
import { ApiError, CoachResponse, describeApiError, chatWithCoach } from "../services/ambatumove";
import { colors, space } from "../theme";

type Turn = { role: "user" | "assistant"; content: string; safety?: CoachResponse["safety_classification"]; actions?: string[]; limitations?: string };

export default function Coach() {
  const s = useStore();
  const [message, setMessage] = useState("");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [consent, setConsent] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [retryQuestion, setRetryQuestion] = useState("");
  const [medicalConcern, setMedicalConcern] = useState(false);
  const scrollRef = useRef<React.ComponentRef<typeof ScrollView>>(null);
  const suggestions = ["Kenapa otot dada kurang terasa saat push-up?", "Bagaimana teknik lat pulldown yang benar?", "Apa perbedaan hammer curl dan bicep curl?", "Berapa lama waktu istirahat antarset?"];
  async function send(questionOverride?: string, retry = false) {
    const question = questionOverride ?? message.trim();
    if (!question || busy) return;
    if (question.length > 2000) {
      setError("Pertanyaan maksimal 2.000 karakter.");
      return;
    }
    if (/\b(nyeri|sakit|cedera|terkilir|bengkak|mati rasa|pusing|sesak|patah|injur|pain|hurt|ache|discomfort|swollen|numb|fractur|bleed|medical)\b/i.test(question)) {
      setMedicalConcern(true);
      setError("");
      setMessage("");
      return;
    }
    if (!consent) {
      setError("Setujui pemrosesan eksternal sebelum meminta jawaban umum dari coach.");
      return;
    }
    setMedicalConcern(false);
    setBusy(true); setError(""); setMessage("");
    const priorTurns = retry ? turns.slice(0, -1) : turns;
    if (!retry) setTurns((current) => [...current, { role: "user", content: question }]);
    try {
      const response = await s.withAuth((token) => chatWithCoach(token, question, consent, priorTurns.slice(-6).map(({ role, content }) => ({ role, content }))));
      setTurns((current) => [...current, { role: "assistant", content: response.answer, safety: response.safety_classification, actions: response.suggested_actions, limitations: response.limitations }]);
      setRetryQuestion("");
    } catch (e) {
      setRetryQuestion(question);
      setError(e instanceof ApiError && e.status === 429 ? "Coach sedang menerima banyak pertanyaan. Tunggu sebentar, lalu coba lagi." : describeApiError(e));
    } finally { setBusy(false); }
  }
  return (
    <Screen scrollViewRef={scrollRef} onContentSizeChange={() => scrollRef.current?.scrollToEnd({ animated: true })} footer={<View style={{ gap: space.sm }}><TextInput accessibilityLabel="Pertanyaan tentang latihan" value={message} onChangeText={setMessage} placeholder="Tulis pertanyaan latihan…" placeholderTextColor={colors.muted} multiline maxLength={2000} editable={!busy} returnKeyType="default" style={[styles.input, { minHeight: 56, maxHeight: 128, textAlignVertical: "top" }]} /><Button title="Kirim pertanyaan" busy={busy} disabled={!message.trim() || busy} onPress={() => void send()} /></View>}>
      <Txt variant="title">Coach latihan</Txt>
      {turns.length > 0 && <Button secondary title="Percakapan baru" disabled={busy} onPress={() => { setTurns([]); setError(""); setRetryQuestion(""); setMessage(""); setMedicalConcern(false); }} />}
      <Notice message="Informasi umum kebugaran, bukan diagnosis atau nasihat medis. Transkrip tidak disimpan oleh layanan. Jangan masukkan nama, kontak, atau informasi pribadi." />
      <Pressable accessibilityRole="checkbox" accessibilityState={{ checked: consent }} onPress={() => setConsent((value) => !value)} style={{ minHeight: 52, flexDirection: "row", alignItems: "center", gap: space.md }}>
        <MaterialCommunityIcons name={consent ? "checkbox-marked-outline" : "checkbox-blank-outline"} size={24} color={consent ? colors.primary : colors.muted} />
        <Txt style={{ flex: 1 }} variant="small">Saya setuju pertanyaan dikirim ke penyedia AI eksternal untuk diproses.</Txt>
      </Pressable>
      <Txt variant="caption" color={colors.muted}>Tanpa persetujuan, AmbatuMove hanya dapat memberi respons keselamatan lokal jika backend mengklasifikasikan pesan sebagai perhatian atau kondisi darurat.</Txt>
      {medicalConcern && <View style={styles.card}><Txt variant="label">Untuk keluhan kesehatan atau cedera</Txt><Txt>Jangan kirim detail medis ke coach AI. Hentikan gerakan yang memicu keluhan dan hubungi tenaga profesional bila perlu.</Txt><Button secondary title="Cari bantuan profesional" onPress={() => router.push("/professionals")} /></View>}
      {error && <Notice error message={`Coach belum dapat menjawab. ${error}`} />}
      {!!error && !!retryQuestion && <Button secondary title="Coba lagi" disabled={busy} onPress={() => void send(retryQuestion, true)} />}
      {turns.length === 0 && <View style={{ gap: space.xs }}>{suggestions.map((question) => <Button key={question} secondary title={question} disabled={busy} onPress={() => { setMessage(question); void send(question); }} />)}</View>}
      {turns.length === 0 ? <View style={styles.emptyState}><Txt variant="heading">Tanya tentang gerakan atau latihan</Txt><Txt color={colors.muted}>Contoh: bagaimana menyesuaikan posisi tubuh saat push-up?</Txt></View> : <View style={{ gap: space.md }}>{turns.map((turn, index) => <View key={`${turn.role}-${index}`} style={[styles.card, { backgroundColor: turn.role === "user" ? colors.primarySoft : colors.surface }]}><Txt variant="label">{turn.role === "user" ? "Anda" : "Coach AmbatuMove"}</Txt><Txt>{turn.content}</Txt>{turn.safety && <Notice message={`Klasifikasi keselamatan: ${turn.safety}`} />}{turn.actions?.map((action, actionIndex) => <Txt key={`${actionIndex}-${action}`}>• {action}</Txt>)}{turn.limitations && <Txt variant="small" color={colors.muted}>{turn.limitations}</Txt>}</View>)}{busy && <View style={styles.card} accessibilityRole="progressbar"><Txt color={colors.muted}>Coach sedang menyiapkan jawaban…</Txt></View>}</View>}
    </Screen>
  );
}
