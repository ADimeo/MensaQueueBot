CREATE TABLE IF NOT EXISTS mensaPreferences (
id INTEGER NOT NULL PRIMARY KEY,
reporterID INTEGER UNIQUE NOT NULL,
wantsMensaMessages INTEGER NOT NULL,
startTimeInUTCMinutes INTEGER, 
endTimeInUTCMinutes INTEGER, 
weekdayBitmap INTEGER,
lastReportDate INTEGER
);
