// ScreenCaptureKit + VideoToolbox köprüsü (CF-235 aday B).
#ifndef CF235_CAPTURE_H
#define CF235_CAPTURE_H

#include <stdint.h>

// İzin durumu: 1 izinli, 0 değil.
int cf_preflight(void);
// İzin ister (sistem penceresi açar), sonucu döner.
int cf_request(void);
// Ana ekranın nokta ve piksel boyutu.
void cf_display_info(int *wpt, int *hpt, int *wpx, int *hpx);
// Yakalamayı başlatır. 0 başarı; hata metni errbuf'a yazılır.
int cf_start(int out_w, int out_h, int fps, int bitrate_kbps, char *errbuf, int errlen);
// Sentetik test deseni ile başlatır (ekran izni gerekmez).
int cf_start_synthetic(int w, int h, int fps, int bitrate_kbps, char *errbuf, int errlen);
void cf_stop(void);
// Bir sonraki kareyi anahtar kare yapar (PLI/FIR gelince).
void cf_force_keyframe(void);
// Süreç bellek bilgisi (bayt): resident ve phys_footprint.
void cf_mem(uint64_t *resident, uint64_t *footprint);

#endif
