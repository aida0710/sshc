set -eu
h=/home/deploy
mkdir -p $h/app/releases/2026-10-29 $h/app/shared/log $h/photos/2026-10 $h/backups $h/docs "$h/設計資料"
magick -size 4032x3024 gradient:'#2b5876-#4e4376' -font /usr/share/fonts/dejavu/DejaVuSans.ttf -fill white -pointsize 160 -gravity center -annotate 0 'IMG_2041 4032x3024' $h/photos/2026-10/IMG_2041.jpg
magick -size 1200x800 plasma:fractal -blur 0x2 $h/photos/2026-10/server-room.png
magick -size 640x480 xc:'#1e293b' -fill '#38bdf8' -draw 'rectangle 40,300 120,440' -draw 'rectangle 160,200 240,440' -draw 'rectangle 280,120 360,440' -draw 'rectangle 400,240 480,440' $h/docs/latency-chart.png
ffmpeg -loglevel error -f lavfi -i testsrc=duration=4:size=640x360:rate=24 -pix_fmt yuv420p $h/photos/2026-10/demo-recording.mp4
for i in $(seq 1 40); do printf '2026-10-29T10:%02d:00Z INFO request id=%04d path=/api/v1/items status=200 dur=%dms\n' $i $i $((i*7%300)) ; done > $h/app/shared/log/app.log
printf 'server {\n  listen 443 ssl;\n  server_name example.internal;\n  root /srv/www;\n}\n' > $h/app/nginx.conf
printf '# 運用メモ\n\n- 再起動の手順\n- バックアップの確認\n' > "$h/設計資料/運用メモ.md"
printf 'id,name,region\n1,web-01,tokyo\n2,web-02,osaka\n' > $h/docs/inventory.csv
head -c 52428800 /dev/urandom > $h/backups/db-2026-10-29.sql.gz
for i in $(seq 1 25); do echo "release $i" > $h/app/releases/2026-10-29/build-$(printf %03d $i).txt; done
echo "export PS1='\\u@web-01:\\w\\$ '" > $h/.bashrc
cp $h/.bashrc $h/.bash_profile
