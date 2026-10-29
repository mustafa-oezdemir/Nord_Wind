# Kuzey Rüzgarları

Almanya pazarı için planlanan, Almanca arayüzlü ve EUR kullanan e-ticaret projesi.

## Mevcut durum

Bu depo şu anda yalnızca başlangıç yapılandırmasını içerir. Angular arayüzü, Go API'si, PostgreSQL şeması ve Docker Compose uygulaması henüz eklenmedi; çalıştırılabilir bir mağaza olduğu varsayılmamalıdır.

## Planlanan temel yapı

- Angular + TypeScript + Bootstrap: müşteri arayüzü
- Go REST API: sunucu uygulaması
- PostgreSQL: kalıcı veri
- Docker Compose ve Caddy: yerel çalışma ve HTTP yönlendirmesi

İlk sürüm tek satıcılı bir mağaza olarak geliştirilecektir. Marka adı **Kuzey Rüzgarları**; teknik adlarda `kuzey-ruzgarlari` kullanılır.

## Yerel ortam

`.env.example` paylaşılabilir anahtarları ve örnek değerleri gösterir. Yerel geliştirme için onu `.env` olarak kopyalayın ve `POSTGRES_PASSWORD` değerini benzersiz, güçlü bir parola ile değiştirin. `.env` ve yerel planlama klasörü `_todo/` Git dışında tutulur.

Uygulama ve Compose dosyaları eklendiğinde çalıştırma ve test komutları burada belgelenecektir.
