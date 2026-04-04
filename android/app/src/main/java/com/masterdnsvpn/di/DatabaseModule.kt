package com.masterdnsvpn.di

import android.content.Context
import androidx.room.Room
import com.masterdnsvpn.profile.AppDatabase
import com.masterdnsvpn.profile.MIGRATION_1_2
import com.masterdnsvpn.profile.MIGRATION_2_3
import com.masterdnsvpn.profile.MIGRATION_3_4
import com.masterdnsvpn.profile.MIGRATION_4_5
import com.masterdnsvpn.profile.MetaProfileDao
import com.masterdnsvpn.profile.ProfileDao
import com.google.gson.Gson
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object DatabaseModule {

    @Provides
    @Singleton
    fun provideDatabase(@ApplicationContext ctx: Context): AppDatabase {
        return Room.databaseBuilder(
            ctx,
            AppDatabase::class.java,
            "masterdnsvpn.db",
        ).addMigrations(MIGRATION_1_2, MIGRATION_2_3, MIGRATION_3_4, MIGRATION_4_5).fallbackToDestructiveMigration().build()
    }

    @Provides
    fun provideProfileDao(db: AppDatabase): ProfileDao = db.profileDao()

    @Provides
    fun provideMetaProfileDao(db: AppDatabase): MetaProfileDao = db.metaProfileDao()

    @Provides
    @Singleton
    fun provideGson(): Gson = Gson()
}