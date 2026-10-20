<!-- release.yml подставляет {{TAG}} (тег релиза) и {{ARCHIVE}} (имя архива: photosync_<версия>_darwin_universal.tar.gz) -->

## Как проверить и запустить

Нужна macOS 13 или новее; архив универсальный, для Apple Silicon и Intel.

1. Скачайте архив и `checksums.txt` в терминале (так macOS не ставит атрибут карантина):

   ```sh
   gh release download {{TAG}} -R Windemiatrix/photosync -p '{{ARCHIVE}}' -p checksums.txt
   ```

   Без `gh`:

   ```sh
   curl -LO https://github.com/Windemiatrix/photosync/releases/download/{{TAG}}/{{ARCHIVE}}
   curl -LO https://github.com/Windemiatrix/photosync/releases/download/{{TAG}}/checksums.txt
   ```

2. Проверьте контрольную сумму. Она защищает только от порчи при скачивании:

   ```sh
   grep '{{ARCHIVE}}' checksums.txt | shasum -a 256 -c -
   ```

3. Проверьте происхождение: подтверждает, что архив собран из этого репозитория workflow релиза. Нужен `gh`; без него подлинность проверить нельзя.

   ```sh
   gh attestation verify '{{ARCHIVE}}' --repo Windemiatrix/photosync
   ```

4. Распакуйте и положите бинарник в `~/bin`:

   ```sh
   tar xzf '{{ARCHIVE}}'
   mkdir -p ~/bin && install -m 755 photosync ~/bin/photosync
   ```

   Если `~/bin` нет в `PATH`, добавьте его или запускайте `~/bin/photosync`.

5. Только если файл скачан браузером и macOS блокирует запуск, и только после проверок выше, снимите карантин с этого одного файла:

   ```sh
   xattr -d com.apple.quarantine ~/bin/photosync
   ```

6. Проверьте установку:

   ```sh
   photosync version
   photosync doctor
   ```

Бинарник подписан только ad-hoc подписью, без подписи Apple и нотаризации. После обновления macOS может заново запросить доступ к «Фото» и Связке ключей: это ожидаемо, `photosync doctor` покажет состояние.

Список изменений ниже — на английском: он собран из заголовков PR.
