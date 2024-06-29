import sys
import os
import json
import subprocess
import requests
import zipfile
import shutil
import platform
import chardet
import logging
from evbunpack.__main__ import unpack_files
from PySide6.QtWidgets import (QApplication, QMainWindow, QPushButton, QFileDialog,
                               QVBoxLayout, QHBoxLayout, QWidget, QMessageBox, QDialog, QComboBox,
                               QDialogButtonBox, QFormLayout, QLabel, QCheckBox, QProgressDialog, QLineEdit)
from PySide6.QtCore import QTimer, QDateTime, Qt

# Set up logging
LOG_FILE = os.path.expanduser("~/Applications/RPGM-Launcher/log.txt")
logging.basicConfig(filename=LOG_FILE, level=logging.DEBUG, 
                    format='%(asctime)s - %(levelname)s - %(message)s')

class FolderPathApp(QMainWindow):
    SETTINGS_FILE = os.path.expanduser("~/Applications/RPGM-Launcher/settings.json")

    def __init__(self):
        super().__init__()

        self.setWindowTitle("RPG Maker MV and MZ Launcher")
        self.resize(400, 200)

        self.central_widget = QWidget()
        self.setCentralWidget(self.central_widget)

        self.layout = QVBoxLayout(self.central_widget)

        version_layout = QHBoxLayout()
        self.version_label = QLabel("NWJS version:", self)
        version_layout.addWidget(self.version_label)

        self.version_selector = QComboBox(self)
        version_layout.addWidget(self.version_selector)
        self.layout.addLayout(version_layout)

        self.extract_checkbox = QCheckBox("Extract game_en.exe", self)
        self.layout.addWidget(self.extract_checkbox)

        self.selected_folder_label = QLabel("No folder selected", self)
        self.layout.addWidget(self.selected_folder_label)

        self.select_button = QPushButton("Select Game Folder", self)
        self.layout.addWidget(self.select_button)

        self.start_game_button = QPushButton("Start Game", self)
        self.layout.addWidget(self.start_game_button)
        self.start_game_button.setEnabled(False)  # Disable until conditions are met

        self.export_button = QPushButton("Export as Standalone App", self)
        self.layout.addWidget(self.export_button)
        self.export_button.setEnabled(False)  # Disable until conditions are met

        self.install_button = QPushButton("Install NWJS Version", self)
        self.layout.addWidget(self.install_button)

        self.uninstall_button = QPushButton("Uninstall NWJS Version", self)
        self.layout.addWidget(self.uninstall_button)

        self.install_button.clicked.connect(self.install_nwjs)
        self.uninstall_button.clicked.connect(self.uninstall_nwjs)
        self.select_button.clicked.connect(self.select_folder)
        self.start_game_button.clicked.connect(self.start_game)
        self.export_button.clicked.connect(self.export_standalone_app)

        self.update_version_selector()
        self.load_settings()
        self.update_select_button_state()
        self.check_start_game_button()

    def load_settings(self):
        if os.path.exists(self.SETTINGS_FILE):
            with open(self.SETTINGS_FILE, 'r') as file:
                settings = json.load(file)
                self.extract_checkbox.setChecked(settings.get('extract_game_en', False))
                last_version = settings.get('last_selected_version')
                if last_version and last_version in [self.version_selector.itemText(i) for i in range(self.version_selector.count())]:
                    self.version_selector.setCurrentText(last_version)
                self.last_selected_folder = settings.get('last_selected_folder', os.path.expanduser("~/Downloads"))
                self.update_selected_folder_label()
        else:
            self.last_selected_folder = os.path.expanduser("~/Downloads")
            self.update_selected_folder_label()

    def save_settings(self):
        settings = {
            'extract_game_en': self.extract_checkbox.isChecked(),
            'last_selected_version': self.version_selector.currentText(),
            'last_selected_folder': self.last_selected_folder
        }
        with open(self.SETTINGS_FILE, 'w') as file:
            json.dump(settings, file, indent=4)

    def closeEvent(self, event):
        self.save_settings()
        super().closeEvent(event)

    def check_nwjs_installed(self):
        applications_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
        return os.path.exists(applications_dir) and any(os.path.isdir(os.path.join(applications_dir, v)) for v in os.listdir(applications_dir))

    def update_version_selector(self):
        applications_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
        if os.path.exists(applications_dir):
            versions = [v for v in os.listdir(applications_dir) if os.path.isdir(os.path.join(applications_dir, v))]
            self.version_selector.clear()
            self.version_selector.addItems(versions)
        self.update_select_button_state()

    def update_select_button_state(self):
        if self.check_nwjs_installed():
            self.select_button.setEnabled(True)
            self.select_button.setText("Select Game Folder")
        else:
            self.select_button.setEnabled(False)
            self.select_button.setText("NWJS not installed")

    def select_folder(self):
        folder_path = QFileDialog.getExistingDirectory(self, "Select Game Folder", self.last_selected_folder)
        if folder_path:
            self.last_selected_folder = folder_path
            logging.info("Selected folder path: %s", folder_path)
            self.update_selected_folder_label()
            if self.check_package_json(folder_path):
                logging.info("Valid RPG Maker MV/MZ game folder.")
                if self.extract_checkbox.isChecked():
                    self.check_and_unpack_game_en(folder_path)
            else:
                logging.error("No package.json found in the selected folder.")
                QMessageBox.critical(self, "Error", "No package.json found in the selected folder.")
        self.save_settings()
        self.check_start_game_button()

    def update_selected_folder_label(self):
        if self.last_selected_folder:
            self.selected_folder_label.setText(f"Selected Folder: {self.truncate_path(self.last_selected_folder)}")
        else:
            self.selected_folder_label.setText("No folder selected")

    def truncate_path(self, path, max_length=50):
        if len(path) > max_length:
            return "..." + path[-(max_length - 3):]
        return path

    def truncate_filename(self, filename, max_length=30):
        if len(filename) > max_length:
            return filename[:max_length // 2] + "..." + filename[-(max_length // 2):]
        return filename

    def check_start_game_button(self):
        if self.last_selected_folder and self.version_selector.currentText():
            self.start_game_button.setEnabled(True)
            self.export_button.setEnabled(True)
        else:
            self.start_game_button.setEnabled(False)
            self.export_button.setEnabled(False)

    def check_package_json(self, folder_path):
        file_path = os.path.join(folder_path, 'package.json')
        if os.path.exists(file_path):
            with open(file_path, 'rb') as file:
                raw_data = file.read()
                result = chardet.detect(raw_data)
                encoding = result['encoding']

            with open(file_path, 'r', encoding=encoding) as file:
                data = json.load(file)

            if 'name' in data and data['name'].strip() == '':
                data['name'] = 'tempname'

            with open(file_path, 'w', encoding=encoding) as file:
                json.dump(data, file, indent=4)

            logging.info("JSON file updated successfully.")
            return True
        else:
            logging.error("File does not exist.")
            return False

    def check_and_unpack_game_en(self, folder_path):
        game_exe = None

        for root, _, files in os.walk(folder_path):
            for file in files:
                if file.lower() == "game_en.exe":
                    game_exe = os.path.join(root, file)
                    break
            if game_exe:
                break

        if game_exe:
            extracted_dir = os.path.join(folder_path, "extracted")
            unpack_files(game_exe, extracted_dir, False, False)

            for item in os.listdir(extracted_dir):
                s = os.path.join(extracted_dir, item)
                d = os.path.join(folder_path, item)
                if os.path.isdir(s):
                    shutil.copytree(s, d, dirs_exist_ok=True)
                else:
                    shutil.copy2(s, d)

            os.remove(game_exe)
            shutil.rmtree(extracted_dir)
            logging.info("game_en.exe extracted and original file removed.")
        else:
            logging.error("game_en.exe not found in the directory")

    def start_game(self):
        folder_path = self.last_selected_folder
        if not folder_path:
            logging.error("No folder selected.")
            QMessageBox.critical(self, "Error", "No folder selected.")
            return

        self.launch_game(folder_path)

    def launch_game(self, folder_path):
        selected_version = self.version_selector.currentText()
        if not selected_version:
            logging.error("No NWJS version selected.")
            QMessageBox.critical(self, "Error", "No NWJS version selected.")
            return

        nwjs_dir = os.path.expanduser(f"~/Applications/RPGM-Launcher/{selected_version}")
        nwjs_path = os.path.join(nwjs_dir, "nwjs.app/Contents/MacOS/nwjs")

        run_with_rosetta_file = os.path.join(nwjs_dir, "run-with-rosetta")
        if os.path.exists(run_with_rosetta_file):
            subprocess.Popen(["arch", "-x86_64", nwjs_path, folder_path])
        else:
            subprocess.Popen([nwjs_path, folder_path])
        logging.info("Game launched using NWJS version %s.", selected_version)

    def export_standalone_app(self):
        app_name, ok = self.get_app_name()
        if not ok:
            return

        destination_folder = QFileDialog.getExistingDirectory(self, "Select Destination Folder")
        if destination_folder:
            try:
                selected_version = self.version_selector.currentText()
                if not selected_version:
                    logging.error("No NWJS version selected.")
                    QMessageBox.critical(self, "Error", "No NWJS version selected.")
                    return

                nwjs_dir = os.path.expanduser(f"~/Applications/RPGM-Launcher/{selected_version}")
                nwjs_app_src = os.path.join(nwjs_dir, "nwjs.app")
                nwjs_app_dst = os.path.join(destination_folder, app_name + ".app")

                # Set up the progress dialog
                total_files = sum([len(files) for _, _, files in os.walk(self.last_selected_folder)])
                progress_dialog = QProgressDialog("Exporting standalone app...", "Cancel", 0, total_files, self)
                progress_dialog.setWindowModality(Qt.WindowModal)
                progress_dialog.setMinimumDuration(0)
                progress_dialog.show()

                shutil.copytree(nwjs_app_src, nwjs_app_dst)

                app_nw_dir = os.path.join(nwjs_app_dst, "Contents", "Resources", "app.nw")
                os.makedirs(app_nw_dir, exist_ok=True)

                current_file_count = 0
                for root, _, files in os.walk(self.last_selected_folder):
                    for file in files:
                        s = os.path.join(root, file)
                        d = os.path.join(app_nw_dir, os.path.relpath(s, self.last_selected_folder))
                        os.makedirs(os.path.dirname(d), exist_ok=True)
                        shutil.copy2(s, d)
                        current_file_count += 1
                        progress_dialog.setValue(current_file_count)
                        progress_dialog.setLabelText(f"Copying over: {self.truncate_filename(file)}")
                        QApplication.processEvents()
                        if progress_dialog.wasCanceled():
                            logging.info("Export canceled by user.")
                            QMessageBox.information(self, "Export Canceled", "Export operation was canceled.")
                            return

                progress_dialog.close()
                QMessageBox.information(self, "Export Complete", "Standalone app exported successfully.")
                logging.info("Standalone app exported successfully to %s", destination_folder)
            except Exception as e:
                logging.error("Error exporting standalone app: %s", str(e))
                QMessageBox.critical(self, "Error", f"Failed to export standalone app: {str(e)}")

    def get_app_name(self):
        dialog = QDialog(self)
        dialog.setWindowTitle("Enter Application Name")
        layout = QVBoxLayout(dialog)

        label = QLabel("Enter a name for the new application:", dialog)
        layout.addWidget(label)

        line_edit = QLineEdit(dialog)
        line_edit.setPlaceholderText("nwjs")
        layout.addWidget(line_edit)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addWidget(buttons)

        result = dialog.exec()
        app_name = line_edit.text().strip() if line_edit.text().strip() else "nwjs"
        return app_name, result == QDialog.Accepted

    def install_nwjs(self):
        URL = "https://nwjs.io/versions"
        BASE_DIR = os.path.expanduser("~/Applications/RPGM-Launcher")

        def download_and_install(version, arch, use_rosetta=False):
            url = f"https://dl.nwjs.io/{version}/nwjs-sdk-{version}-osx-{arch}.zip"
            tmp_file = "/tmp/nwjs.zip"

            logging.info("Downloading and installing version %s for %s architecture...", version, arch)
            response = requests.get(url, stream=True)
            if response.status_code != 200:
                logging.error("Failed to download version %s for %s.", version, arch)
                return

            total_size = int(response.headers.get('content-length', 0))
            downloaded_size = 0
            chunk_size = 1024  # 1KB

            progress_dialog = QProgressDialog("Downloading NWJS...", "Cancel", 0, total_size, self)
            progress_dialog.setWindowModality(Qt.WindowModal)
            progress_dialog.setMinimumDuration(0)
            progress_dialog.show()

            start_time = QDateTime.currentDateTime()
            canceled = False

            with open(tmp_file, "wb") as f:
                for chunk in response.iter_content(chunk_size):
                    if chunk:
                        f.write(chunk)
                        downloaded_size += len(chunk)
                        progress_dialog.setValue(downloaded_size)

                        elapsed_time = start_time.msecsTo(QDateTime.currentDateTime()) / 1000
                        download_speed = downloaded_size / (1024 * 1024) / elapsed_time  # MB/s

                        progress_dialog.setLabelText(f"Downloaded: {downloaded_size / (1024 * 1024):.2f} MB of {total_size / (1024 * 1024):.2f} MB\n"
                                                     f"Speed: {download_speed:.2f} MB/s")

                        if progress_dialog.wasCanceled():
                            canceled = True
                            logging.info("Download canceled by user.")
                            break

            if canceled:
                os.remove(tmp_file)
                QMessageBox.information(self, "NWJS Installation", "NWJS Installation Canceled.")
                return

            subprocess.run(["unzip", "-q", tmp_file, "-d", "/tmp"])

            target_dir = os.path.join(BASE_DIR, version)
            if os.path.exists(target_dir):
                shutil.rmtree(target_dir)
            
            os.makedirs(target_dir, exist_ok=True)
            shutil.move(f"/tmp/nwjs-sdk-{version}-osx-{arch}/nwjs.app", target_dir)
            logging.info("Version %s installed successfully at %s", version, target_dir)

            if use_rosetta:
                with open(os.path.join(target_dir, "run-with-rosetta"), "w") as f:
                    f.write("This file indicates NWJS should be run with Rosetta 2.")

            os.remove(tmp_file)
            shutil.rmtree(f"/tmp/nwjs-sdk-{version}-osx-{arch}")

            progress_dialog.close()
            QMessageBox.information(self, "Install NWJS Version", "NWJS installation completed successfully.")

        arch = "arm64" if platform.machine() == "arm64" else "x64"
        logging.info("Querying available versions...")
        response = requests.get(URL)
        if response.status_code != 200:
            logging.error("Failed to query NWJS versions.")
            return

        data = response.json()
        versions = {v["version"]: v for v in data["versions"]}
        
        version, ok = self.show_version_selection_dialog(versions.keys())
        if ok and version:
            version_info = versions[version]
            use_rosetta = False
            if arch == "arm64" and "osx-arm64" not in version_info["files"]:
                use_rosetta = self.show_rosetta_warning()
                if not use_rosetta:
                    return
                arch = "x64"
            download_and_install(version, arch, use_rosetta)
            self.save_settings()
            self.update_version_selector()
            self.load_settings()
            self.check_start_game_button()

    def show_version_selection_dialog(self, versions):
        dialog = QDialog(self)
        dialog.setWindowTitle("Select NWJS Version")
        layout = QFormLayout(dialog)
        
        combo_box = QComboBox(dialog)
        combo_box.addItems(versions)
        layout.addRow("Version:", combo_box)

        buttons = QDialogButtonBox(QDialogButtonBox.Ok | QDialogButtonBox.Cancel, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addRow(buttons)

        result = dialog.exec()
        return combo_box.currentText(), result == QDialog.Accepted

    def show_rosetta_warning(self):
        dialog = QDialog(self)
        dialog.setWindowTitle("Rosetta Warning")
        layout = QVBoxLayout(dialog)

        label = QLabel("An Apple Silicon / ARM64 build of this version of NWJS is unavailable. Would you like to use the Intel / x64 version with Rosetta?", dialog)
        layout.addWidget(label)

        buttons = QDialogButtonBox(QDialogButtonBox.Yes | QDialogButtonBox.No, dialog)
        buttons.accepted.connect(dialog.accept)
        buttons.rejected.connect(dialog.reject)
        layout.addWidget(buttons)

        result = dialog.exec()
        return result == QDialog.Accepted

    def uninstall_nwjs(self):
        applications_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
        versions = [v for v in os.listdir(applications_dir) if os.path.isdir(os.path.join(applications_dir, v))]

        version, ok = self.show_version_selection_dialog(versions)
        if ok and version:
            target_dir = os.path.join(applications_dir, version)
            if os.path.exists(target_dir):
                shutil.rmtree(target_dir)
                self.save_settings()
                self.update_version_selector()
                self.load_settings()
                self.update_select_button_state()
                self.check_start_game_button()
                logging.info("NWJS version %s uninstalled successfully.", version)
                QMessageBox.information(self, "Uninstall NWJS Version", f"NWJS version {version} uninstalled successfully.")
            else:
                logging.error("NWJS version %s not found.", version)
                QMessageBox.critical(self, "Error", f"NWJS version {version} not found.")

def check_appdir():
    app_dir = os.path.expanduser("~/Applications/RPGM-Launcher")
    if not os.path.exists(app_dir):
        os.makedirs(app_dir)
    # Clear log file on startup
    with open(LOG_FILE, 'w') as file:
        file.write("")

def main():
    check_appdir()
    
    # Redirect stdout and stderr to log file
    sys.stdout = open(LOG_FILE, 'a')
    sys.stderr = open(LOG_FILE, 'a')
    
    app = QApplication(sys.argv)
    window = FolderPathApp()
    window.show()
    sys.exit(app.exec())

if __name__ == "__main__":
    main()
