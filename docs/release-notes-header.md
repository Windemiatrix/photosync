<!-- Шапка заметок к релизу: GoReleaser исполняет файл как шаблон Go (make release склеивает его с английской версией). Допустимы только поля Tag и Version, другой логики шаблонов нет. -->

## Как проверить и запустить

Нужна macOS 13 или новее; архив универсальный, для Apple Silicon и Intel.

1. Скачайте архив и `checksums.txt` в терминале (так macOS не ставит атрибут карантина):

   ```sh
   gh release download {{ .Tag }} -R Windemiatrix/photosync -p 'photosync_{{ .Version }}_darwin_universal.tar.gz' -p checksums.txt
   ```

   Без `gh`:

   ```sh
   curl -LO https://github.com/Windemiatrix/photosync/releases/download/{{ .Tag }}/photosync_{{ .Version }}_darwin_universal.tar.gz
   curl -LO https://github.com/Windemiatrix/photosync/releases/download/{{ .Tag }}/checksums.txt
   ```

2. Проверьте контрольную сумму. Она защищает только от порчи при скачивании:

   ```sh
   grep 'photosync_{{ .Version }}_darwin_universal.tar.gz' checksums.txt | shasum -a 256 -c -
   ```

3. Проверьте происхождение: подтверждает, что архив собран из этого репозитория workflow релиза. Нужен `gh`; без него подлинность проверить нельзя.

   ```sh
   gh attestation verify 'photosync_{{ .Version }}_darwin_universal.tar.gz' --repo Windemiatrix/photosync
   ```

4. Распакуйте и положите бинарник в `~/bin`:

   ```sh
   tar xzf 'photosync_{{ .Version }}_darwin_universal.tar.gz'
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

Бинарник подписан только ad-hoc подписью, без подписи Apple и нотаризации. Процесс, запущенный через LaunchAgent, после обновления может заново запросить доступ к «Фото» (при запуске из терминала право сохраняется): это ожидаемо, `photosync doctor` покажет состояние.

Список изменений ниже — на английском: он собран из заголовков PR.
